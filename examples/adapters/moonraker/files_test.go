package moonraker_test

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"example.com/prusa/moonraker"
	"example.com/prusa/moonraker/moonrakersim"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

type memoryContent struct {
	data []byte
}

func (m memoryContent) Size() int64 { return int64(len(m.data)) }

func (m memoryContent) Open(_ context.Context, offset int64) (io.ReadCloser, error) {
	if offset >= int64(len(m.data)) {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	return io.NopCloser(bytes.NewReader(m.data[offset:])), nil
}

type blockingContent struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	data    []byte
}

func (b *blockingContent) Size() int64 { return int64(len(b.data)) }

func (b *blockingContent) Open(_ context.Context, _ int64) (io.ReadCloser, error) {
	b.once.Do(func() { close(b.started) })

	return &blockingReader{release: b.release, r: bytes.NewReader(b.data)}, nil
}

type blockingReader struct {
	release chan struct{}
	r       io.Reader
}

func (b *blockingReader) Read(p []byte) (int, error) {
	<-b.release

	return b.r.Read(p)
}

func (b *blockingReader) Close() error { return nil }

func TestMoonrakerStoragesAndListings(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	machine.PutFile("benchy.gcode", []byte("G28\n"))
	machine.PutFile("projects/part.gcode", []byte("G1 X10\n"))
	machine.PutFile(".thumbs/benchy-32x32.png", []byte("png"))
	machine.PutFile(".thumbs/benchy-300x300.png", []byte("root-thumb"))
	machine.PutFile("projects/.thumbs/part-300x300.png", []byte("sub-thumb"))

	session := connect(t, machine)

	files, ok := connectorapi.Files(session)
	require.True(t, ok)

	storages, err := files.Storages(t.Context())
	require.NoError(t, err)
	require.Len(t, storages, 1)
	require.Equal(t, "gcodes", storages[0].Name)
	require.Equal(t, "/gcodes", storages[0].Mountpoint)
	require.True(t, storages[0].FreeSpaceKnown)
	require.False(t, storages[0].ReadOnly)

	root, err := files.List(t.Context(), "")
	require.NoError(t, err)
	require.True(t, root.Directory.Folder)
	require.Len(t, root.Entries, 1)
	require.Equal(t, "gcodes", root.Entries[0].Path)

	gcodes, err := files.List(t.Context(), "gcodes")
	require.NoError(t, err)
	require.True(t, gcodes.Directory.Folder)
	require.Equal(t, "gcodes", gcodes.Directory.Path)

	paths := make([]string, 0, len(gcodes.Entries))
	for _, entry := range gcodes.Entries {
		paths = append(paths, entry.Path)
	}

	require.Contains(t, paths, "gcodes/projects")
	require.Contains(t, paths, "gcodes/benchy.gcode")
	require.NotContains(t, paths, "gcodes/.thumbs")

	rootSingle, err := files.List(t.Context(), "gcodes/benchy.gcode")
	require.NoError(t, err)
	require.Equal(t, []byte("root-thumb"), rootSingle.Directory.Preview)

	single, err := files.List(t.Context(), "gcodes/projects/part.gcode")
	require.NoError(t, err)
	require.False(t, single.Directory.Folder)
	require.Equal(t, "gcodes/projects/part.gcode", single.Directory.Path)
	require.Equal(t, int64(len("G1 X10\n")), single.Directory.Size)
	require.Equal(t, []byte("sub-thumb"), single.Directory.Preview)

	_, err = files.List(t.Context(), "gcodes/missing.gcode")
	require.ErrorIs(t, err, connectorapi.ErrListingRefused)
	require.ErrorContains(t, err, "Metadata not available for requested file")
}

func TestMoonrakerFileManagerAndStartPrint(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)

	manager, ok := connectorapi.Manages(session)
	require.True(t, ok)

	transfers, ok := connectorapi.Uploads(session)
	require.True(t, ok)

	require.NoError(t, manager.CreateFolder(t.Context(), "gcodes/batch"))

	payload := bytes.Repeat([]byte("G1 X1 Y1\n"), 1024)

	var reports []connectorapi.Progress
	err := transfers.Send(t.Context(), connectorapi.Upload{
		Path:     "gcodes/batch/bracket.gcode",
		Content:  memoryContent{data: payload},
		Progress: func(p connectorapi.Progress) { reports = append(reports, p) },
	})
	require.NoError(t, err)
	require.NotEmpty(t, reports)
	require.Equal(t, int64(len(payload)), reports[len(reports)-1].Sent)
	require.True(t, reports[len(reports)-1].Known)

	stored, exists := machine.FileBytes("batch/bracket.gcode")
	require.True(t, exists)
	require.Equal(t, payload, stored)

	result, err := session.Do(t.Context(), connectorapi.Command{
		Verb: moonraker.VerbStartPrint,
		Args: map[string]string{"path": "gcodes/batch/bracket.gcode"},
	})
	require.NoError(t, err)
	require.True(t, result.OK)
	require.Equal(t, []string{"batch/bracket.gcode"}, machine.StartedFiles())

	require.NoError(t, manager.DeleteFile(t.Context(), "gcodes/batch/bracket.gcode"))
	_, exists = machine.FileBytes("batch/bracket.gcode")
	require.False(t, exists)

	require.NoError(t, manager.DeleteFolder(t.Context(), "gcodes/batch"))
}

func TestMoonrakerUploadBusyAndMismatch(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)
	transfers, ok := connectorapi.Uploads(session)
	require.True(t, ok)

	blocking := &blockingContent{
		started: make(chan struct{}),
		release: make(chan struct{}),
		data:    []byte("G28\n"),
	}

	firstErr := make(chan error, 1)
	go func() {
		firstErr <- transfers.Send(t.Context(), connectorapi.Upload{
			Path:    "gcodes/first.gcode",
			Content: blocking,
		})
	}()

	<-blocking.started

	secondErr := transfers.Send(t.Context(), connectorapi.Upload{
		Path:    "gcodes/second.gcode",
		Content: memoryContent{data: []byte("G28\n")},
	})
	require.ErrorIs(t, secondErr, connectorapi.ErrTransferBusy)

	close(blocking.release)
	require.NoError(t, <-firstErr)

	corruptMachine := startMachine(t, moonrakersim.Options{CorruptUploadSize: true})
	corruptSession := connect(t, corruptMachine)
	corruptTransfers, ok := connectorapi.Uploads(corruptSession)
	require.True(t, ok)

	err := corruptTransfers.Send(t.Context(), connectorapi.Upload{
		Path:    "gcodes/bad.gcode",
		Content: memoryContent{data: []byte("G28\nG29\n")},
	})
	require.Error(t, err)

	var faultErr *fault.Fault
	require.ErrorAs(t, err, &faultErr)
	require.Equal(t, connectorapi.TransferNotOnStorage.Code(), faultErr.Code())
}

func TestMoonrakerStorageCapabilitiesRespectMachineState(t *testing.T) {
	t.Parallel()

	noGcodes := connect(t, startMachine(t, moonrakersim.Options{WithoutGcodes: true}))
	caps, _ := noGcodes.Capabilities()
	require.False(t, caps.Has(moonraker.VerbSendInfo))
	require.False(t, caps.Has(moonraker.VerbSendFileInfo))
	require.False(t, caps.Has(moonraker.VerbFileUpload))
	require.False(t, caps.Has(moonraker.VerbStartPrint))

	readOnly := connect(t, startMachine(t, moonrakersim.Options{GcodesReadOnly: true}))
	roCaps, _ := readOnly.Capabilities()
	require.True(t, roCaps.Has(moonraker.VerbSendInfo))
	require.True(t, roCaps.Has(moonraker.VerbSendFileInfo))
	require.False(t, roCaps.Has(moonraker.VerbFileUpload))
	require.False(t, roCaps.Has(moonraker.VerbCreateFolder))
	require.False(t, roCaps.Has(moonraker.VerbDeleteFile))
	require.False(t, roCaps.Has(moonraker.VerbDeleteFolder))
	require.True(t, roCaps.Has(moonraker.VerbStartPrint))

	noSD := connect(t, startMachine(t, moonrakersim.Options{
		Objects: without(moonrakersim.DefaultObjects(), "virtual_sdcard"),
	}))
	noSDCaps, _ := noSD.Capabilities()
	require.True(t, noSDCaps.Has(moonraker.VerbSendFileInfo))
	require.False(t, noSDCaps.Has(moonraker.VerbStartPrint))
}

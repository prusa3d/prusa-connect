package xbuddy

// A firmware flash, which the firmware takes only as the G-code M997.
//
// Prusa-Firmware-Buddy has no FLASH or UPGRADE JSON command
// (src/connect/command.cpp) and answers either with "Unknown command". M997
// names the .bbf by its 8.3 short name in the root of the stick and resets the
// board at once, whatever the printer is doing (PrusaGcodeSuite::M997 in
// src/marlin_stubs/M997.cpp). Prusa Connect sends both verbs this way and only
// in the states START_PRINT is accepted in (FlashWithPath in its commands.py).

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strings"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

const (
	flashGCode = "M997"
	// flashRoot is the one storage M997 reads a file from. The firmware looks
	// for this exact prefix, so it is not the storage INFO names.
	flashRoot = "/usb/"
	// shortNameMax is the longest 8.3 name, and what M997 copies after flashRoot.
	shortNameMax = 12

	statusConflict = 409
)

// moduleWord is an S and a digit inside a name. Marlin's parser finds it in the
// path as M997's module number, and a module other than 0 flashes nothing and
// still answers FINISHED.
var moduleWord = regexp.MustCompile(`S[0-9]`)

// flashNotSent is a flash this driver did not send, because the printer did not
// answer the question that names the image by its short name.
//
//nolint:gochecknoglobals // a registered code, read-only after init.
var flashNotSent = fault.Register(
	fault.DomainPrinter(), "FLASH_NOT_SENT", statusConflict, fault.Document(
		"The printer did not say the firmware file's short name, so the server did not send the flash",
		"Nothing was flashed. Send the flash again once the printer answers. The message says what "+
			"the printer did instead of answering.",
		fault.RetryAllowed,
		fault.ShowDetail,
		"verb",
		"path",
	))

// gcodeFlash renders M997 for the file the path names, or a bare M997, which
// makes the firmware flash the newest image on the stick when it restarts. An
// empty path reaches here as the root of the storage (see absolutePath), so the
// root is the bare form too.
//
// A path M997 would misread is refused here rather than flashing nothing.
func gcodeFlash(args arguments) (string, error) {
	path, named := args.text("path")
	if !named || path == "" || path+"/" == flashRoot || path == flashRoot {
		return flashGCode, nil
	}

	name, onStick := strings.CutPrefix(path, flashRoot)
	if !onStick || name == "" || strings.Contains(name, "/") || len(name) > shortNameMax ||
		moduleWord.MatchString(name) {
		return "", fmt.Errorf("%w: the firmware flashes only a file in the root of %s named by an 8.3 "+
			"short name with no S before a digit, and %q is not one", ErrArgumentUnusable, flashRoot, path)
	}

	return flashGCode + " " + path, nil
}

// shortNamed returns a flash with its path in the 8.3 spelling M997 reads, and
// any other command as it came.
//
// Every path this driver hands out uses long names (see rooted), and the
// printer's FILE_INFO for the file is what names its short one. A refusal of
// that question is the printer's answer to the flash as well, so it is returned
// with answered set. A FILE_INFO that names no path leaves the path as it was,
// and gcodeFlash refuses it unless it is already a short name.
//
// No path, or the stick itself, is the bare M997, and needs no question.
func (s *Session) shortNamed(
	ctx context.Context, cmd connectorapi.Command,
) (named connectorapi.Command, answer connectorapi.Result, answered bool, err error) {
	if cmd.Verb != VerbFlash && cmd.Verb != VerbUpgrade {
		return cmd, connectorapi.Result{}, false, nil
	}

	path := cmd.Args["path"]
	if path == "" || absolutePath(path, s.storage) == s.storage {
		cmd.Args = maps.Clone(cmd.Args)
		delete(cmd.Args, "path")

		return cmd, connectorapi.Result{}, false, nil
	}

	result, err := s.Do(ctx, connectorapi.Command{
		Verb: VerbSendFileInfo,
		Args: map[string]string{"path": path},
	})
	if err != nil {
		// Not wrapped: the question's own outcome, such as COMMAND_UNANSWERED,
		// would be read as the flash's, and no M997 left.
		return cmd, connectorapi.Result{}, true, flashNotSent.
			New("the printer did not answer which short name "+path+" has: "+err.Error()).
			With(fault.StringParam("verb", cmd.Verb.String()), fault.StringParam("path", path))
	}

	if !result.OK {
		return cmd, result, true, nil
	}

	if short, ok := shortPathOf(result.Raw); ok {
		cmd.Args = maps.Clone(cmd.Args)
		cmd.Args["path"] = short
	}

	return cmd, connectorapi.Result{}, false, nil
}

// shortPathOf is the 8.3 path a FILE_INFO event names its node by.
func shortPathOf(raw json.RawMessage) (string, bool) {
	var frame fileInfoFrame
	if json.Unmarshal(raw, &frame) != nil || frame.Event != eventFileInfo || frame.Data.Path == "" {
		return "", false
	}

	return frame.Data.Path, true
}

// refuseInState stops a stateChecked verb before it reaches the wire when the
// state the printer reported on its current connection is not in its
// stateGuards row. A printer that has not reported one there yet is UNKNOWN.
func (s *Session) refuseInState(verb connectorapi.Capability) error {
	if !wireTable[verb].stateChecked {
		return nil
	}

	state := s.heardState()
	if acceptedIn(verb, state) {
		return nil
	}

	return fault.PrinterVerbNotAllowedInState.
		New("the printer reported "+string(state)+", and "+verb.String()+" is not sent in that state").
		With(fault.StringParam("verb", verb.String()), fault.StringParam("state", string(state)))
}

// settledOnWrite settles a settlesOnWrite verb whose frame has just left, and
// reports whether it did.
func (s *Session) settledOnWrite(pending *call) bool {
	if !wireTable[pending.verb].settlesOnWrite {
		return false
	}

	s.forget(pending.id)
	pending.settle(outcome{result: connectorapi.Result{OK: true, Ignored: pending.dropped}})

	return true
}

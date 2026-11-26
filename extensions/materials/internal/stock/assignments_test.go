package stock

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"

	"github.com/prusa3d/materials/internal/dbtest"
)

func TestAssignThenUnassign(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			s, ctx := newStore(t, engine), context.Background()
			spool := addSpools(t, s, "alice", "m-a", 1, 800)[0]

			got, err := s.Assign(
				ctx,
				"alice",
				Assignment{DeviceID: "d1", ToolID: "0", SpoolID: &spool.ID, By: ByPerson},
			)
			if err != nil {
				t.Fatal(err)
			}

			if got.MaterialUUID != "m-a" || got.MaterialSource != "catalogue" || got.MaterialName != "PLA" ||
				got.UpdatedAt.IsZero() {
				t.Fatalf("the spool's material was not copied: %+v", got)
			}

			mine, err := s.CreateCustom(ctx, "alice", MaterialFields{Name: "Mine", Class: "FFF", Abbreviation: "MYPLA"})
			if err != nil {
				t.Fatal(err)
			}

			reported := json.RawMessage(`{"uuid":"tag"}`)
			if _, err = s.Assign(ctx, "alice", Assignment{
				DeviceID: "d1", ToolID: "1", MaterialUUID: mine.UUID, By: ByPerson, Reported: reported,
			}); err != nil {
				t.Fatal(err)
			}

			list, err := s.Assignments(ctx, "d1")
			if err != nil || len(list) != 2 {
				t.Fatalf("%+v %v", list, err)
			}

			if a := list[1]; a.ToolID != "1" || a.MaterialSource != "custom" || a.Abbreviation != "MYPLA" ||
				a.SpoolID != nil || string(a.Reported) != string(reported) {
				t.Fatalf("%+v", a)
			}

			if on := s.toolOf(t, spool.ID); on == nil || on.DeviceID != "d1" || on.ToolID != "0" {
				t.Fatalf("spool on %+v", on)
			}

			// Someone else's custom material cannot be picked.
			if _, err = s.Assign(
				ctx,
				"bob",
				Assignment{DeviceID: "d1", ToolID: "1", MaterialUUID: mine.UUID, By: ByPerson},
			); !errors.Is(
				err,
				ErrNotFound,
			) {
				t.Fatalf("bob's pick: %v", err)
			}

			if err = s.Unassign(ctx, "d1", "0"); err != nil {
				t.Fatal(err)
			}

			if on := s.toolOf(t, spool.ID); on != nil {
				t.Fatalf("the spool is still on a tool: %+v", on)
			}

			if list, err = s.Assignments(ctx, "d1"); err != nil || len(list) != 1 || list[0].ToolID != "1" {
				t.Fatalf("%+v %v", list, err)
			}

			// A tool with nothing on it unassigns too.
			if err = s.Unassign(ctx, "d1", "0"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAssignRefusesWhatCannotBeAssigned(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			s, ctx := newStore(t, engine), context.Background()
			spool := addSpools(t, s, "alice", "m-a", 1, 800)[0]
			missing := int64(9999)

			for name, c := range map[string]struct {
				a    Assignment
				want error
			}{
				"no material":      {Assignment{DeviceID: "d1", ToolID: "0", By: ByPerson}, ErrInvalid},
				"no device":        {Assignment{ToolID: "0", MaterialUUID: "m-a", By: ByPerson}, ErrInvalid},
				"no tool":          {Assignment{DeviceID: "d1", MaterialUUID: "m-a", By: ByPerson}, ErrInvalid},
				"nobody":           {Assignment{DeviceID: "d1", ToolID: "0", MaterialUUID: "m-a"}, ErrInvalid},
				"unknown material": {Assignment{DeviceID: "d1", ToolID: "0", MaterialUUID: "zzz", By: ByPerson}, ErrNotFound},
				"unknown spool":    {Assignment{DeviceID: "d1", ToolID: "0", SpoolID: &missing, By: ByPerson}, ErrNotFound},
			} {
				if _, err := s.Assign(ctx, "alice", c.a); !errors.Is(err, c.want) {
					t.Errorf("%s: want %v, got %v", name, c.want, err)
				}
			}

			if err := s.DeleteSpool(ctx, spool.ID, t0); err != nil {
				t.Fatal(err)
			}

			if _, err := s.Assign(
				ctx,
				"alice",
				Assignment{DeviceID: "d1", ToolID: "0", SpoolID: &spool.ID, By: ByPerson},
			); !errors.Is(
				err,
				ErrNotFound,
			) {
				t.Fatalf("a deleted spool: %v", err)
			}
		})
	}
}

func TestOneSpoolOnOneToolUnderARace(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			s, ctx := newStore(t, engine), context.Background()
			spool := addSpools(t, s, "alice", "m-a", 1, 800)[0]

			var (
				wg   sync.WaitGroup
				errs [2]error
			)

			for i, tool := range []string{"0", "1"} {
				wg.Go(func() {
					_, errs[i] = s.Assign(
						ctx,
						"alice",
						Assignment{DeviceID: "d1", ToolID: tool, SpoolID: &spool.ID, By: ByPerson},
					)
				})
			}

			wg.Wait()

			refused := 0

			for _, err := range errs {
				switch {
				case errors.Is(err, ErrSpoolOnAnotherTool):
					refused++
				case err != nil:
					t.Fatal(err)
				}
			}

			if refused != 1 {
				t.Fatalf("%d assigns were refused: %v", refused, errs)
			}

			if n := count(t, s, "assignments"); n != 1 {
				t.Fatalf("%d assignments", n)
			}

			// Assigning the spool again to the tool it is on changes nothing.
			if _, err := s.Assign(
				ctx,
				"alice",
				Assignment{DeviceID: "d1", ToolID: s.toolOf(t, spool.ID).ToolID, SpoolID: &spool.ID, By: ByPerson},
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAnAssignedSpoolKeepsItsWeightAndStock(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			s, ctx := newStore(t, engine), context.Background()
			spools := addSpools(t, s, "alice", "m-a", 2, 800)

			if _, err := s.Assign(
				ctx,
				"alice",
				Assignment{DeviceID: "d1", ToolID: "0", SpoolID: &spools[0].ID, By: ByPerson},
			); err != nil {
				t.Fatal(err)
			}

			// An archived spool goes on no tool.
			if _, err := s.UpdateSpool(ctx, spools[0].ID, SpoolPatch{Archived: new(true)}); !errors.Is(
				err,
				ErrSpoolAssigned,
			) {
				t.Fatalf("archive: %v", err)
			}

			if err := s.DeleteSpool(ctx, spools[0].ID, t0); !errors.Is(err, ErrSpoolAssigned) {
				t.Fatalf("delete: %v", err)
			}

			if err := s.DeleteSpools(ctx, []int64{spools[1].ID, spools[0].ID}, t0); !errors.Is(err, ErrSpoolAssigned) {
				t.Fatalf("delete both: %v", err)
			}

			page, err := s.Spools(ctx, "m-a", 0, 0)
			if err != nil || page.Total != 2 || page.Spools[0].RemainingWeightG != 800 ||
				page.Spools[0].ArchivedAt != nil {
				t.Fatalf("a refused change was kept: %+v %v", page, err)
			}

			// Its notes and where it is kept change while it is on the tool.
			if got, noteErr := s.UpdateSpool(ctx, spools[0].ID, SpoolPatch{Notes: new("dry")}); noteErr != nil ||
				got.Notes != "dry" {
				t.Fatalf("notes of a spool on a tool: %+v %v", got, noteErr)
			}

			if err = s.Unassign(ctx, "d1", "0"); err != nil {
				t.Fatal(err)
			}

			if _, err = s.UpdateSpool(ctx, spools[0].ID, SpoolPatch{Notes: new("moved")}); err != nil {
				t.Fatal(err)
			}

			if err = s.DeleteSpools(ctx, []int64{spools[1].ID, spools[0].ID}, t0); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAPersonsAssignKeepsTheReport(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			s, ctx := newStore(t, engine), context.Background()
			reported := json.RawMessage(`{"instance_uuid":"tag"}`)

			if _, err := s.Assign(ctx, "", Assignment{
				DeviceID: "d1", ToolID: "0", Abbreviation: "PLA", By: ByPrinter, Reported: reported,
			}); err != nil {
				t.Fatal(err)
			}

			if _, err := s.Assign(ctx, "alice", Assignment{
				DeviceID: "d1", ToolID: "0", MaterialUUID: "m-a", By: ByPerson,
			}); err != nil {
				t.Fatal(err)
			}

			list, err := s.Assignments(ctx, "d1")
			if err != nil || len(list) != 1 || list[0].By != ByPerson || string(list[0].Reported) != string(reported) {
				t.Fatalf("%+v %v", list, err)
			}
		})
	}
}

// The follower reads a tool, decides and then writes. A person's assign that
// lands between the read and the write stands.
func TestAPrintersReportYieldsToAPersonsAssign(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			s, ctx := newStore(t, engine), context.Background()
			spool := addSpools(t, s, "alice", "m-a", 1, 800)[0]

			first := Assignment{DeviceID: "d1", ToolID: "0", Abbreviation: "PETG"}
			if ok, err := s.Report(ctx, first, nil); err != nil || !ok {
				t.Fatalf("the first report: %v %v", ok, err)
			}

			read := s.mustAssignments(t, "d1")[0]

			if _, err := s.Assign(ctx, "alice", Assignment{
				DeviceID: "d1", ToolID: "0", MaterialUUID: "m-a", By: ByPerson,
			}); err != nil {
				t.Fatal(err)
			}

			stale := read
			stale.Abbreviation = "ASA"

			if err := s.Refresh(ctx, stale); err != nil {
				t.Fatal(err)
			}

			if err := s.Drop(ctx, read); err != nil {
				t.Fatal(err)
			}

			for _, guard := range []*time.Time{&read.UpdatedAt, nil} {
				late := Assignment{DeviceID: "d1", ToolID: "0", Abbreviation: "ASA"}
				if ok, err := s.Report(ctx, late, guard); err != nil || ok {
					t.Fatalf("a stale report was written: %v %v", ok, err)
				}
			}

			if got := s.mustAssignments(t, "d1"); len(got) != 1 || got[0].By != ByPerson || got[0].Abbreviation != "" ||
				got[0].MaterialUUID != "m-a" {
				t.Fatalf("the person's assign did not stand: %+v", got)
			}

			// A refused report that would move a spool leaves it where it is.
			if _, err := s.Assign(ctx, "alice", Assignment{
				DeviceID: "d2", ToolID: "0", SpoolID: &spool.ID, By: ByPerson,
			}); err != nil {
				t.Fatal(err)
			}

			taken := Assignment{DeviceID: "d1", ToolID: "0", SpoolID: &spool.ID}
			if ok, err := s.Report(ctx, taken, nil); err != nil || ok {
				t.Fatalf("%v %v", ok, err)
			}

			if on := s.toolOf(t, spool.ID); on == nil || on.DeviceID != "d2" || on.ToolID != "0" {
				t.Fatalf("the spool left its tool: %+v", on)
			}

			// A report that is written moves it.
			free := Assignment{DeviceID: "d1", ToolID: "1", SpoolID: &spool.ID}
			if ok, err := s.Report(ctx, free, nil); err != nil || !ok {
				t.Fatalf("%v %v", ok, err)
			}

			if on := s.toolOf(t, spool.ID); on == nil || on.DeviceID != "d1" || on.ToolID != "1" {
				t.Fatalf("the spool did not move: %+v", on)
			}
		})
	}
}

func (s *Store) mustAssignments(t *testing.T, device string) []Assignment {
	t.Helper()

	got, err := s.Assignments(context.Background(), device)
	if err != nil {
		t.Fatal(err)
	}

	return got
}

func TestAStoredReportKeepsItsOwnNames(t *testing.T) {
	r := connectorapi.ReportedMaterial{
		InstanceUUID: "i-1", PackageUUID: "p-1", MaterialUUID: "m-1", Type: "PLA", ColorRGBA: "#ff0000ff",
		FullWeight: new(1000.0), Properties: map[string]float64{"min_print_temperature": 190},
	}

	raw, err := EncodeReport(r)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"instance_uuid":"i-1","package_uuid":"p-1","material_uuid":"m-1","type":"PLA",` +
		`"color_rgba":"#ff0000ff","full_weight_g":1000,"properties":{"min_print_temperature":190}}`
	if string(raw) != want {
		t.Errorf("the report is stored as %s, want %s", raw, want)
	}

	back, err := DecodeReport(raw)
	if err != nil || !reflect.DeepEqual(back, r) {
		t.Errorf("the stored report reads back as %+v (%v), want %+v", back, err, r)
	}

	if back, err = DecodeReport(nil); err != nil || !reflect.DeepEqual(back, connectorapi.ReportedMaterial{}) {
		t.Errorf("no stored report reads as %+v (%v)", back, err)
	}
}

// toolOf answers the tool a spool is on, nil for none.
func (s *Store) toolOf(t *testing.T, id int64) *ToolRef {
	t.Helper()

	sp, err := s.spool(context.Background(), s.db, id)
	if err != nil {
		t.Fatal(err)
	}

	return sp.Tool
}

func TestAssignedSpoolsReadsThePrintersAskedWithTheirSpools(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			s, ctx := newStore(t, engine), context.Background()
			spool := addSpools(t, s, "alice", "m-a", 1, 800)[0]

			for _, a := range []Assignment{
				{DeviceID: "d1", ToolID: "0", SpoolID: &spool.ID, By: ByPerson},
				{DeviceID: "d1", ToolID: "1", MaterialUUID: "m-a", By: ByPerson},
				{DeviceID: "d2", ToolID: "0", MaterialUUID: "m-a", By: ByPerson},
			} {
				if _, err := s.Assign(ctx, "alice", a); err != nil {
					t.Fatal(err)
				}
			}

			got, err := s.AssignedSpools(ctx, []string{"d1", "d3"})
			if err != nil {
				t.Fatal(err)
			}

			if len(got) != 2 || got[0].ToolID != "0" || got[1].ToolID != "1" {
				t.Fatalf("want d1's two tools only, got %+v", got)
			}

			if got[0].SpoolUUID != spool.UUID || got[0].RemainingWeightG == nil || *got[0].RemainingWeightG != 800 {
				t.Fatalf("the spool on d1/0 was not read: %+v", got[0])
			}

			if got[1].SpoolUUID != "" || got[1].RemainingWeightG != nil {
				t.Fatalf("a tool with no spool read one: %+v", got[1])
			}

			if none, noneErr := s.AssignedSpools(ctx, nil); noneErr != nil || len(none) != 0 {
				t.Fatalf("no printers: %v, %v", none, noneErr)
			}
		})
	}
}

func TestATaggedSpoolWithNoWeightTakesOneOnATool(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			s, ctx := listStore(t, engine), context.Background()
			used := 595.0

			// The printer reported the tag's used weight and no full weight,
			// and the catalogue has no package of the material.
			sp, err := s.AddReported(ctx, "alice",
				ReportedSpool{UUID: "tag-1", MaterialUUID: "m-2", Abbreviation: "PLA", ConsumedG: &used}, 10)
			if err == nil {
				_, err = s.Assign(ctx, "alice", Assignment{DeviceID: "d1", ToolID: "0", SpoolID: &sp.ID, By: ByPrinter})
			}

			if err != nil {
				t.Fatal(err)
			}

			// The weight typed and the grams the tag says were used make its
			// full weight.
			got, err := s.UpdateSpool(ctx, sp.ID, SpoolPatch{RemainingWeightG: new(405.0)})
			if err != nil || got.RemainingWeightG != 405 || got.FullWeightG == nil || *got.FullWeightG != 1000 {
				t.Fatalf("the weight of a tagged spool with none: %+v %v", got, err)
			}

			// Its archive stays the printer's.
			if _, err = s.UpdateSpool(ctx, sp.ID, SpoolPatch{Archived: new(true)}); !errors.Is(err, ErrSpoolAssigned) {
				t.Fatalf("archive: %v", err)
			}

			// A spool with a full weight takes a corrected weight and keeps
			// its full weight.
			full := 1000.0

			weighed, err := s.AddReported(ctx, "alice",
				ReportedSpool{UUID: "tag-2", MaterialUUID: "m-1", Abbreviation: "PLA", FullWeightG: &full}, 10)
			if err == nil {
				_, err = s.Assign(
					ctx,
					"alice",
					Assignment{DeviceID: "d1", ToolID: "1", SpoolID: &weighed.ID, By: ByPrinter},
				)
			}

			if err != nil {
				t.Fatal(err)
			}

			got, err = s.UpdateSpool(ctx, weighed.ID, SpoolPatch{RemainingWeightG: new(5.0)})
			if err != nil || got.RemainingWeightG != 5 || got.FullWeightG == nil || *got.FullWeightG != 1000 {
				t.Fatalf("the weight of a tagged spool with one: %+v %v", got, err)
			}
		})
	}
}

// A spool added at 0 g before its printer reported a weight takes the weight
// when it is added again, and a spool with a weight keeps it.
func TestAddingATaggedSpoolAgainWeighsIt(t *testing.T) {
	for _, engine := range dbtest.Engines(t) {
		t.Run(string(engine.Name), func(t *testing.T) {
			s, ctx := listStore(t, engine), context.Background()
			used, full := 595.0, 1000.0

			sp, err := s.AddReported(ctx, "alice",
				ReportedSpool{UUID: "tag-1", MaterialUUID: "m-2", Abbreviation: "PLA", ConsumedG: &used}, 10)
			if err != nil || sp.FullWeightG != nil || sp.RemainingWeightG != 0 {
				t.Fatalf("added with no weight: %+v %v", sp, err)
			}

			again := ReportedSpool{
				UUID: "tag-1", MaterialUUID: "m-2", Abbreviation: "PLA", FullWeightG: &full, ConsumedG: &used,
			}

			sp, err = s.AddReported(ctx, "alice", again, 10)
			if err != nil || sp.FullWeightG == nil || *sp.FullWeightG != 1000 || sp.RemainingWeightG != 405 {
				t.Fatalf("added again with a weight: %+v %v", sp, err)
			}

			// A weight a person typed stays.
			if _, err = s.UpdateSpool(ctx, sp.ID, SpoolPatch{RemainingWeightG: new(300.0)}); err != nil {
				t.Fatal(err)
			}

			if sp, err = s.AddReported(ctx, "alice", again, 10); err != nil || sp.RemainingWeightG != 300 {
				t.Fatalf("added again after a typed weight: %+v %v", sp, err)
			}
		})
	}
}

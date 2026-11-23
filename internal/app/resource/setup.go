package resource

import (
	"errors"
	"fmt"
)

// The two first-run modes an operator can record. There are only two: a
// deployment asks every caller for a credential, or it serves every request
// as the configured principal.
const (
	// SetupOpen serves every request as core.principal and asks for no
	// credential.
	SetupOpen = "open"
	// SetupRequired asks every caller for a credential.
	SetupRequired = "required"
)

// The two choices of the first run for how a new printer joins. There are
// only two. They are terms of the core and not of one dialect: a connector
// translates them into the credential its own wire checks.
const (
	// AdmissionAsk makes a new printer wait. It registers, shows a code on
	// its screen, and an operator approves that code.
	AdmissionAsk = "ask"
	// AdmissionOpen admits a printer as soon as it dials. It is for a trusted
	// local network.
	AdmissionOpen = "open"
)

// EffectiveAdmission is the choice a printer is admitted on. An install that
// has recorded none uses AdmissionAsk, so a printer that dials before anybody
// has chosen is not admitted without approval.
func EffectiveAdmission(choice string) string {
	if choice == "" {
		return AdmissionAsk
	}

	return choice
}

// ErrSetupRecorded reports a first-run mode offered for an install that
// already recorded one. If the API accepted a second one, anybody who can
// reach the port could change the mode.
var ErrSetupRecorded = errors.New("resource: a first-run decision is already recorded")

// SetupMode returns the first-run mode the overlay records, and empty when
// the install has recorded none.
//
// It reads the file and not the last reconciled document, because it is
// called on the start-up path, before the first reconcile.
func (s *Store) SetupMode() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	overlay, err := s.readOverlay()
	if err != nil {
		return "", err
	}

	if overlay.Auth == nil {
		return "", nil
	}

	return overlay.Auth.Mode, nil
}

// RecordSetupMode writes the first-run mode into the overlay and refuses to
// replace a recorded one.
//
// It reads the file, sets the one key and writes the file back under the
// store's lock, so it does not lose an instance that an earlier write
// added.
func (s *Store) RecordSetupMode(mode string) error {
	if mode != SetupOpen && mode != SetupRequired {
		return fmt.Errorf("%w: the first-run mode is %q, expected %q or %q",
			ErrConfigInvalid, mode, SetupOpen, SetupRequired)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.updateOverlay(func(overlay *overlayFile) error {
		if overlay.Auth != nil && overlay.Auth.Mode != "" {
			return fmt.Errorf("%w: %s", ErrSetupRecorded, overlay.Auth.Mode)
		}

		overlay.Auth = &overlayAuth{Mode: mode}

		return nil
	})
}

// AdmissionMode returns how a new printer joins, as the overlay records it,
// and empty for an install that has recorded nothing.
//
// It reads the file for the same reason as SetupMode: the choice is read on
// the start-up path, before the connectors are mounted and before the first
// reconcile.
func (s *Store) AdmissionMode() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	overlay, err := s.readOverlay()
	if err != nil {
		return "", err
	}

	if overlay.Admission == nil {
		return "", nil
	}

	return overlay.Admission.Mode, nil
}

// RecordAdmission writes how a new printer joins into the overlay and changes
// no other member of the file.
//
// It replaces a recorded choice, and RecordSetupMode does not. The first run
// writes the admission choice first and the mode after it. If the first run
// fails between the two, it must be able to write the admission choice again
// on the next try.
func (s *Store) RecordAdmission(mode string) error {
	if mode != AdmissionAsk && mode != AdmissionOpen {
		return fmt.Errorf("%w: the admission choice is %q, expected %q or %q",
			ErrConfigInvalid, mode, AdmissionAsk, AdmissionOpen)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.updateOverlay(func(overlay *overlayFile) error {
		overlay.Admission = &overlayAdmission{Mode: mode}

		return nil
	})
}

// DiscoveryHidden returns the hidden setting the overlay records. recorded is
// false while nobody has used the switch, and the caller then takes
// core.discovery.hidden from the operator's file.
func (s *Store) DiscoveryHidden() (hidden, recorded bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	overlay, err := s.readOverlay()
	if err != nil {
		return false, false, err
	}

	if overlay.Discovery == nil {
		return false, false, nil
	}

	return overlay.Discovery.Hidden, true, nil
}

// RecordDiscoveryHidden writes the hidden setting into the overlay and
// changes no other member of the file. It replaces a recorded value, because
// a person may change the setting as often as they like.
func (s *Store) RecordDiscoveryHidden(hidden bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.updateOverlay(func(overlay *overlayFile) error {
		overlay.Discovery = &overlayDiscovery{Hidden: hidden}

		return nil
	})
}

// TimeServerEnabled returns the time server switch the overlay records.
// recorded is false while nobody has used the switch, and the caller then
// takes core.time_server.enabled from the operator's file.
func (s *Store) TimeServerEnabled() (enabled, recorded bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	overlay, err := s.readOverlay()
	if err != nil {
		return false, false, err
	}

	if overlay.TimeServer == nil {
		return false, false, nil
	}

	return overlay.TimeServer.Enabled, true, nil
}

// RecordTimeServerEnabled writes the time server switch into the overlay and
// changes no other member of the file.
func (s *Store) RecordTimeServerEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.updateOverlay(func(overlay *overlayFile) error {
		overlay.TimeServer = &overlayTimeServer{Enabled: enabled}

		return nil
	})
}

// readOverlay parses the machine-owned file from disk. An absent file is an
// empty overlay, as on a first run.
func (s *Store) readOverlay() (overlayFile, error) {
	content, err := readOptional(s.paths.Overlay)
	if err != nil {
		return overlayFile{}, err
	}

	return parseOverlay(content)
}

package httpapi

import "time"

// admissionExamples is the worked answer for the event stream's reader count
// and for the operator's half of admission. They are in this file because
// examples.go is at its length limit.
func admissionExamples() map[string]operationExample {
	return map[string]operationExample{
		"get-event-stream-stats": {Response: EventStreamStats{Subscribers: 3}},

		"list-pending-registrations": {Response: map[string]any{
			"items": []Registration{exampleRegistration()},
		}},

		"approve-registration": {Response: approvedExample()},

		"start-printer-pairing": {
			Status: "201",
			Request: PrinterPairingRequest{
				Printer: "192.168.1.20",
				Connect: PrinterPairingConnect{Host: "192.168.1.50", Port: 8080},
			},
			Response: waitingPairingExample(),
		},
		"get-printer-pairing":    {Response: admittedPairingExample()},
		"cancel-printer-pairing": {Response: withdrawnPairingExample()},

		"set-device-tags": {
			Request:  map[string]any{"tags": []string{"workshop", "PETG only", "night shift"}},
			Response: map[string]any{"tags": []string{"workshop", "PETG only", "night shift"}},
		},

		"set-device-labels": {
			Request:  map[string]any{"labels": map[string]string{"site": "brno", "spare": ""}},
			Response: map[string]any{"labels": map[string]string{"site": "brno", "spare": ""}},
		},
	}
}

// exampleRegistration is one device waiting for a decision, as the listing
// shows it. The code is drawn from the alphabet an operator can read aloud, and
// the row carries no credential.
func exampleRegistration() Registration {
	return Registration{
		Code:        exampleRegistrationCode,
		Connector:   "xbuddy",
		Fingerprint: "b3f1c0d2e4a5",
		Serial:      "8221-1234567",
		Model:       "MK4",
		Firmware:    "6.1.3",
	}
}

// approvedExample is the same registration once somebody has let it through.
//
// It carries the device id the approval bound the machine to, which is what
// separates the answer to an approval from a row on the listing: a page reads
// it and opens that device's page.
func approvedExample() Registration {
	approved := exampleRegistration()
	approved.Approved = true
	approved.DeviceID = examplePrinterID

	return approved
}

// waitingPairingExample is a pairing whose question is on the printer's
// screen. The code is drawn from the alphabet the firmware takes.
func waitingPairingExample() PrinterPairing {
	started := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	until := started.Add(75 * time.Second)

	return PrinterPairing{
		ID:        examplePrinterPairingID,
		Code:      "K7QX-29FM",
		Printer:   "192.168.1.20:80",
		Connect:   PrinterPairingConnect{Host: "192.168.1.50", Port: 8080},
		State:     "waiting",
		StartedAt: started,
		Until:     &until,
	}
}

// admittedPairingExample is the same pairing once the printer's registration
// was approved, with the device the page opens next.
func admittedPairingExample() PrinterPairing {
	admitted := waitingPairingExample()
	admitted.State = "admitted"
	admitted.Until = nil
	admitted.Registration = &PairedRegistration{
		Code:      exampleRegistrationCode,
		Connector: "xbuddy",
		Serial:    "8221-1234567",
		Model:     "MK4",
		DeviceID:  examplePrinterID,
	}

	return admitted
}

func withdrawnPairingExample() PrinterPairing {
	withdrawn := waitingPairingExample()
	withdrawn.State = "withdrawn"
	withdrawn.Until = nil

	return withdrawn
}

// examplePrinterPairingID is the id a started pairing answers with.
const examplePrinterPairingID = "4f2a9c1e7b3d4a6f8e0c2b5d7a9f1e3c"

package httpapi

// discoveryExamples are the worked examples for the printers nearby and the
// hidden setting. One story runs through them. A workshop has two printers on
// one network, and this server hears both. The MK4 is already connected to this
// server, so its entry names it. The workshop has hidden this server, so the
// others do not list it.
func discoveryExamples() map[string]operationExample {
	return map[string]operationExample{
		"list-nearby-printers": {Response: map[string]any{"items": []NearbyPrinter{
			{
				Name:     "Workshop Core One",
				Address:  "192.168.1.20",
				Port:     443,
				TLS:      true,
				Services: []string{"_prusa-connect._tcp", "_prusalink._tcp"},
				URL:      "https://192.168.1.20:443/",
			},
			{
				Name:     "Workshop MK4",
				Address:  "192.168.1.21",
				Port:     80,
				Services: []string{"_prusalink._tcp"},
				URL:      "http://192.168.1.21:80/",
				DeviceID: "0199e2c1-0405-7e48-b0af-e812537ad808",
			},
		}}},

		"get-discovery-visibility": {Response: hiddenWorkshopServer()},

		"get-time-server": {Response: workshopTimeServer()},

		"set-time-server": {
			Request:  map[string]any{"enabled": true},
			Response: workshopTimeServer(),
		},

		"list-allowed-origins": {Response: map[string]any{"items": workshopOrigins()}},

		"set-allowed-origins": {
			Request:  map[string]any{"origins": []string{"https://dashboard.workshop.example"}},
			Response: map[string]any{"items": workshopOrigins()},
		},

		"set-discovery-visibility": {
			Request:  map[string]any{"hidden": true},
			Response: hiddenWorkshopServer(),
		},
	}
}

// hiddenWorkshopServer is this server in the workshop of the story above,
// which the workshop has hidden.
func hiddenWorkshopServer() Visibility {
	return Visibility{
		Hidden:       true,
		Announcement: "hidden",
		Name:         "Workshop",
		Host:         "workshop-server.local",
		Port:         8080,
		Services:     []string{"_prusalink._tcp", "_prusa-connect._tcp"},
		Addresses:    []string{"192.168.1.10", "fe80::1c2b:3aff:fe4d:5e6f"},
	}
}

// workshopOrigins is the pages the workshop lets call this server: its own
// dashboard, which a person added in Settings, and the site of a slicer
// extension that is switched on.
func workshopOrigins() []AllowedOrigin {
	return []AllowedOrigin{
		{Origin: "https://dashboard.workshop.example", Source: OriginFromSettings},
		{
			Origin: "https://slicer.example", Source: OriginFromExtension,
			Extension: "com.github.acme.slicer", Title: "Web slicer",
		},
	}
}

// workshopTimeServer is the workshop's server answering the time, because the
// workshop network has no internet.
func workshopTimeServer() TimeServerState {
	return TimeServerState{Enabled: true, State: "serving", Port: timeServerPort}
}

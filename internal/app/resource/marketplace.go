package resource

// MarketplaceEnabled returns the marketplace switch the overlay records.
// recorded is false while nobody has used the switch, and the caller then
// reads the marketplace core.marketplace.url names whenever one is named.
func (s *Store) MarketplaceEnabled() (enabled, recorded bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	overlay, err := s.readOverlay()
	if err != nil {
		return false, false, err
	}

	if overlay.Marketplace == nil || overlay.Marketplace.Enabled == nil {
		return false, false, nil
	}

	return *overlay.Marketplace.Enabled, true, nil
}

// RecordMarketplaceEnabled writes the marketplace switch into the overlay and
// changes no other member of the file. It replaces a recorded value, because
// a person may switch the marketplace off and on as often as they like.
func (s *Store) RecordMarketplaceEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.updateOverlay(func(overlay *overlayFile) error {
		if overlay.Marketplace == nil {
			overlay.Marketplace = &overlayMarketplace{}
		}

		overlay.Marketplace.Enabled = &enabled

		return nil
	})
}

// MarketplaceCommunity returns whether a person chose to list the community
// extensions. It is false until somebody does.
func (s *Store) MarketplaceCommunity() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	overlay, err := s.readOverlay()
	if err != nil {
		return false, err
	}

	return overlay.Marketplace != nil && overlay.Marketplace.Community, nil
}

// RecordMarketplaceCommunity writes the community switch into the overlay and
// changes no other member of the file, the marketplace switch included.
func (s *Store) RecordMarketplaceCommunity(shown bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.updateOverlay(func(overlay *overlayFile) error {
		if overlay.Marketplace == nil {
			overlay.Marketplace = &overlayMarketplace{}
		}

		overlay.Marketplace.Community = shown

		return nil
	})
}

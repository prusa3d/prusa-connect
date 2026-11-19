#!/bin/sh
# Refuses a raw search box in a Svelte file. Every search box in a listing is
# a ListingSearch, which carries the clear button and the Escape key,
# so a hand-written `type="search"` input would lack them.
# `make frontend-check` runs it. The other raw elements are refused by ESLint;
# see "Primitives" in .agents/rules/ui-integration.md.
set -e

# Files allowed to hold a raw search box, one per line, each with its reason.
EXEMPT='
packages/svelte/src/primitives/listing-search/listing-search.svelte
packages/svelte/src/primitives/select-facet/select-facet.svelte
extensions/materials/ui/src/add-dialog/choose-material.svelte
extensions/parts/ui/src/cards/chosen-card-editor.svelte
packages/svelte/src/shell/printer-detail/filament-position.svelte
extensions/materials/ui/src/printer-material/material-picker.svelte
'
# listing-search.svelte: the primitive itself.
# select-facet.svelte: the search inside a facet panel.
# choose-material.svelte, chosen-card-editor.svelte, filament-position.svelte:
#   pickers, not listings.
# material-picker.svelte: a selector that focuses the ListingSearch input it
#   draws, which holds no input of its own.

cd "$(dirname "$0")/.."
found=$(grep -rnE "type=[\"']search[\"']" --include='*.svelte' \
	frontend/src packages/svelte/src extensions/*/ui/src 2>/dev/null || true)
bad=$(printf '%s\n' "$found" | while IFS= read -r l; do
	[ -n "$l" ] || continue
	f=${l%%:*}
	printf '%s\n' "$EXEMPT" | grep -qxF "$f" || printf '%s\n' "$l"
done)
if [ -n "$bad" ]; then
	echo "A raw search box; use ListingSearch instead:"
	printf '%s\n' "$bad"
	exit 1
fi

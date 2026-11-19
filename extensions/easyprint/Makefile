# The gates of the easyprint module. extensions/Makefile runs `check` here,
# and so does `make -C extensions check EXT=easyprint`.

# The module is a browser bundle alone, with no Go half to lint.
NO_GO_MODULE = 1

include ../extension.mk

.PHONY: check

# Lint, type-check and test the bundle, then build it and install the release
# into a scratch core. See README.md.
check: lint
	bash scripts/check.sh

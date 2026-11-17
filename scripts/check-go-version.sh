#!/usr/bin/env bash
# The Go toolchain pin, held against every place that repeats it and against the
# newest release the repository knows about.
#
# The rule is the latest release, once it has been published for MIN_AGE_DAYS
# days.
#
# The default mode reads only files in the tree, so it runs on an air-gapped
# builder. scripts/go-releases.tsv is the record of which releases exist and
# when each was published; --refresh rewrites it from go.dev and needs network.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INVENTORY_REL="scripts/go-releases.tsv"
INVENTORY="$ROOT/$INVENTORY_REL"
MIN_AGE_DAYS=5
FAILED=0

fail() { echo "FAIL $*" >&2; FAILED=1; }
ok()   { echo "ok   $*"; }

# Days since 1970-01-01 for YYYY-MM-DD, so two dates compare without a clock
# and without GNU date.
days() {
  awk -v d="$1" 'BEGIN{
    split(d,p,"-"); y=p[1]; m=p[2]; dd=p[3];
    if (m<=2) { y-=1; m+=12 }
    era = int((y>=0?y:y-399)/400);
    yoe = y - era*400;
    doy = int((153*(m-3)+2)/5) + dd-1;
    doe = yoe*365 + int(yoe/4) - int(yoe/100) + doy;
    print era*146097 + doe - 719468;
  }'
}

# 1 when $1 is a strictly newer Go version than $2.
newer() {
  awk -v a="$1" -v b="$2" 'BEGIN{
    na=split(a,x,"."); nb=split(b,y,".");
    for (i=1;i<=3;i++) {
      ai=(i<=na?x[i]+0:0); bi=(i<=nb?y[i]+0:0);
      if (ai>bi) { print 1; exit } if (ai<bi) { print 0; exit }
    }
    print 0;
  }'
}

refresh() {
  echo "Refreshing $INVENTORY_REL from go.dev ..."
  local versions tmp
  # go.dev without include=all lists the supported releases, which is the
  # newest patch of each supported minor. That is the whole question the rule
  # asks, so the inventory stays small and the refresh stays one request per
  # release.
  versions="$(curl -fsS --max-time 60 'https://go.dev/dl/?mode=json' \
    | grep -o '"version": *"go[0-9][^"]*"' \
    | sed 's/^.*"go//; s/"$//' \
    | grep -E '^[0-9]+\.[0-9]+(\.[0-9]+)?$' | sort -u)"
  [ -n "$versions" ] || { echo "FAIL go.dev returned no stable versions" >&2; exit 1; }
  tmp="$(mktemp)"
  {
    echo "# Go releases this repository knows about, and the day each was published."
    echo "# Rewritten by scripts/check-go-version.sh --refresh, which needs network."
    echo "# The published date is the golang.org/toolchain module's proxy timestamp."
    echo "generated	$(date -u +%Y-%m-%d)"
    # A hold is a judgement, not a fact about the world, so a refresh carries
    # the existing ones forward rather than discarding them.
    if [ -f "$INVENTORY" ]; then grep '^hold	' "$INVENTORY" || true; fi
  } > "$tmp"
  local v when want got missing
  want=0
  missing=""
  for v in $versions; do
    want=$((want + 1))
    when="$(curl -fsS --max-time 30 \
      "https://proxy.golang.org/golang.org/toolchain/@v/v0.0.1-go$v.linux-amd64.info" \
      | grep -o '"Time":"[0-9-]*' | cut -c9- || true)"
    if [ -n "$when" ]; then printf '%s\t%s\n' "$v" "$when" >> "$tmp"
    else missing="$missing $v"
    fi
  done

  # The listing above is armed and the timestamps below were not, so a run that
  # reached go.dev and no proxy rewrote the inventory with zero releases and
  # exited 0. A short file reads exactly like a complete one, and the release it
  # dropped is the release the next run judges the pin against. So a refresh
  # that could not answer for every release it listed writes nothing.
  got="$(grep -cvE '^#|^generated	|^hold	' "$tmp" || true)"
  if [ -n "$missing" ] || [ "$got" -lt "$want" ] || [ "$want" -lt 1 ]; then
    echo "FAIL the proxy answered for $got of $want releases, missing:${missing:- none}" >&2
    echo "     $INVENTORY_REL is unchanged, because a partial inventory is worse" >&2
    echo "     than a stale one. Re-run --refresh when the network is back." >&2
    rm -f "$tmp"
    exit 1
  fi

  grep -vE '^#|^generated	|^hold	' "$tmp" | sort -Vr > "$tmp.s"
  { grep -E '^#|^generated	|^hold	' "$tmp"; cat "$tmp.s"; } > "$INVENTORY"
  rm -f "$tmp" "$tmp.s"
  echo "Wrote $(grep -cvE '^#|^generated	|^hold	' "$INVENTORY") releases."
}

[ "${1:-}" = "--refresh" ] && refresh

PIN="$(awk '/^go /{print $2; exit}' "$ROOT/go.mod")"
[ -n "$PIN" ] || { echo "FAIL go.mod has no go directive" >&2; exit 1; }
echo "go.mod pins Go $PIN"

# Every other place the version is written has to agree with go.mod, because a
# build that reads the stale copy compiles on a toolchain nobody reviewed.
SDK_PIN="$(awk '/^go /{print $2; exit}' "$ROOT/sdk/go.mod")"
[ "$SDK_PIN" = "$PIN" ] && ok "sdk/go.mod" || fail "sdk/go.mod pins $SDK_PIN"

for f in docker/server/Dockerfile .github/dockerfiles/Dockerfile.build docker/mockup/Dockerfile; do
  ARG_PIN="$(grep -m1 '^ARG GO_VERSION=' "$ROOT/$f" | cut -d= -f2)"
  [ "$ARG_PIN" = "$PIN" ] && ok "$f" || fail "$f has ARG GO_VERSION=$ARG_PIN"
done

# The race container (docker/race, through make test-race-linux) builds FROM a
# golang image pinned by digest, with the tag beside the digest. The tag is
# what a reader sees, so it has to name the release go.mod pins: a digest that
# outlives a Go bump is an operating system layer nobody re-pinned.
PIN_MINOR="$(echo "$PIN" | cut -d. -f1,2)"
# A FROM line may carry a --platform flag before the image, as a stage that
# cross-compiles does. A file with no golang FROM line leaves the value empty,
# and the comparison below then fails with a message instead of a silent exit.
FROM_GOLANG='^FROM (--platform=[^ ]+ )?golang:'
RACE_PIN="$(grep -m1 -E "$FROM_GOLANG" "$ROOT/docker/race/Dockerfile" \
  | sed -E 's/^FROM (--platform=[^ ]+ )?golang:([0-9]+\.[0-9]+\.[0-9]+).*/\2/' || true)"
[ "$RACE_PIN" = "$PIN" ] && ok "docker/race/Dockerfile" \
  || fail "docker/race/Dockerfile builds FROM golang:$RACE_PIN, go.mod says $PIN"

# The builder stages take their tag from ARG GO_VERSION and carry a digest
# too. A digest wins over the tag, so each one has to name the race image's
# digest, which the check above ties to the release go.mod pins.
RACE_DIGEST="$(grep -m1 -E "$FROM_GOLANG" "$ROOT/docker/race/Dockerfile" \
  | sed -E 's/.*@(sha256:[0-9a-f]+).*/\1/' || true)"
for f in docker/server/Dockerfile .github/dockerfiles/Dockerfile.build docker/mockup/Dockerfile; do
  BUILDER_DIGEST="$(grep -m1 -E "$FROM_GOLANG" "$ROOT/$f" | sed -E 's/.*@(sha256:[0-9a-f]+).*/\1/' || true)"
  [ "$BUILDER_DIGEST" = "$RACE_DIGEST" ] && ok "$f golang digest" \
    || fail "$f builds FROM golang@$BUILDER_DIGEST, docker/race/Dockerfile names $RACE_DIGEST"
done

# README.md is the copy a person building from source reads, and no gate held
# it: its requirements list stated Go 1.26.5 against a go.mod pinning 1.26.7.
#
# The reader is every `Go x.y.z` in the file rather than one line number, so a
# second mention added later is held the day it appears. A README that names no
# version fails rather than passing, because a check with nothing to compare is
# the failure this repository keeps finding.
README_PINS="$(grep -oE 'Go [0-9]+\.[0-9]+\.[0-9]+' "$ROOT/README.md" \
  | awk '{print $2}' | sort -u || true)"
if [ -z "$README_PINS" ]; then
  fail "README.md names no Go version, so this check read nothing."
  echo "     The requirements list carries it. Point this check at it if it moved." >&2
elif [ "$README_PINS" = "$PIN" ]; then
  ok "README.md"
else
  fail "README.md names Go $(printf '%s' "$README_PINS" | tr '\n' ' '), go.mod pins $PIN"
fi

# The other page a stranger reads before building. It names the minor release
# and not the patch, which is the weaker claim and the right one for a page
# telling somebody which command to type, so it is held against PIN_MINOR.
GS="docs/src/content/docs/getting-started.md"
GS_PINS="$(grep -oE 'Go [0-9]+\.[0-9]+' "$ROOT/$GS" | awk '{print $2}' | sort -u || true)"
if [ -z "$GS_PINS" ]; then
  fail "$GS names no Go version, so this check read nothing."
  echo "     The table of three commands carries it. Point this check at it if it moved." >&2
elif [ "$GS_PINS" = "$PIN_MINOR" ]; then
  ok "$GS"
else
  fail "$GS names Go $(printf '%s' "$GS_PINS" | tr '\n' ' '), go.mod pins $PIN_MINOR"
fi

# The page an extension author reads before scaffolding a driver. It names the
# patch release, because the scaffold copies that number out of sdk/go.mod into
# the go.mod it writes, and an author whose toolchain is older than it cannot
# build what the tool just wrote. The page stated 1.26.5 against an sdk
# declaring 1.26.8, which is the failure this reader is here for.
WD="docs/src/content/docs/writing-a-driver.md"
WD_PINS="$(grep -oE 'Go [0-9]+\.[0-9]+\.[0-9]+' "$ROOT/$WD" | awk '{print $2}' | sort -u || true)"
if [ -z "$WD_PINS" ]; then
  fail "$WD names no Go version, so this check read nothing."
  echo "     The toolchain table carries it. Point this check at it if it moved." >&2
elif [ "$WD_PINS" = "$PIN" ]; then
  ok "$WD"
else
  fail "$WD names Go $(printf '%s' "$WD_PINS" | tr '\n' ' '), go.mod pins $PIN"
fi

# The rule about "latest" answered from a file rather than from the network.
# A release counts once it has been published for five days by the inventory's
# own generated date, so the answer does not move between two runs of the same
# checkout.
#
# A `hold` line takes one release out of the running and says what blocks it.
# The rule still applies to every other release, so a hold on 1.27.0 does not
# excuse the tree from 1.26.8, and deleting the line puts 1.27.0 back.
[ -f "$INVENTORY" ] || { echo "FAIL $INVENTORY_REL is missing" >&2; exit 1; }
GENERATED="$(awk '/^generated\t/{print $2; exit}' "$INVENTORY")"
GEN_DAY="$(days "$GENERATED")"
BEST=""
RELEASES=0
while IFS=$'\t' read -r v when reason; do
  case "$v" in ''|\#*|generated) continue;; esac
  if [ "$v" = hold ]; then
    echo "held Go $when is not adopted: $reason"
    continue
  fi
  RELEASES=$((RELEASES + 1))
  awk -F'\t' -v want="$v" '$1=="hold" && $2==want {found=1} END{exit !found}' "$INVENTORY" && continue
  age=$(( GEN_DAY - $(days "$when") ))
  [ "$age" -ge "$MIN_AGE_DAYS" ] || continue
  if [ -z "$BEST" ] || [ "$(newer "$v" "$BEST")" = 1 ]; then BEST="$v"; fi
done < "$INVENTORY"
[ "$RELEASES" -gt 0 ] || { echo "FAIL $INVENTORY_REL lists no releases" >&2; exit 1; }
# go.dev lists only the newest patch of each minor, so for five days after a
# patch release a refreshed inventory holds nothing old enough. Nothing is
# then owed, and the pin stays.
if [ -z "$BEST" ]; then
  ok "no release in $INVENTORY_REL has cleared $MIN_AGE_DAYS days, so Go $PIN stays"
elif [ "$(newer "$BEST" "$PIN")" = 1 ]; then
  fail "Go $BEST has been out $MIN_AGE_DAYS days or more and the tree pins $PIN."
  echo "     Bump the go directive in go.mod and sdk/go.mod, then rerun this check." >&2
else
  ok "Go $PIN is the newest release in $INVENTORY_REL that clears $MIN_AGE_DAYS days"
fi

exit $FAILED

#!/bin/sh
# Weekly recap of what the agent did and what waits on a person, over several
# target repositories on one machine. No model: jalon list, jq, gh and git.
#
#   weekly-recap.sh [-days N] [-metrics FILE] [-notify CMD] REPO...
#
# -days     the window, default 7
# -metrics  the JALON_METRICS file, default $JALON_METRICS
# -notify   a command that receives the recap on stdin (jalon knows one command,
#           not one chat service); absent, the recap goes to stdout only
#
# Every section names a decision a person can take from a phone: a task doing
# for too long, an agreed task nobody queued, a pull request nobody read, a
# wreck nobody removed, a decision whose files moved since it was taken. The
# two numbers of the work kill criterion close it. What is not here is a
# proposal: those come from measurements, and this reads them.
set -eu

days=7
metrics=${JALON_METRICS-}
notify=
while [ $# -gt 0 ]; do
  case "$1" in
    -days) days=$2; shift 2 ;;
    -metrics) metrics=$2; shift 2 ;;
    -notify) notify=$2; shift 2 ;;
    -*) echo "weekly-recap: unknown flag $1" >&2; exit 2 ;;
    *) break ;;
  esac
done
[ $# -gt 0 ] || { echo "usage: weekly-recap.sh [-days N] [-metrics FILE] [-notify CMD] REPO..." >&2; exit 2; }
for tool in jalon jq gh git; do
  command -v "$tool" >/dev/null 2>&1 || { echo "weekly-recap: $tool is not on PATH" >&2; exit 1; }
done

since=$(date -u -d "$days days ago" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-"$days"d +%Y-%m-%dT%H:%M:%SZ)
stale_days=14
today=$(date -u +%Y-%m-%d)

# epoch YYYY-MM-DD, GNU date first, BSD date as the fallback for a laptop.
epoch() {
  date -u -d "$1" +%s 2>/dev/null || date -u -j -f %Y-%m-%d "$1" +%s
}

# age_days ID: days since the date carried by a jalon id (YYMMDD-slug).
age_days() {
  d=$(printf %s "$1" | sed -E 's/^([0-9]{2})([0-9]{2})([0-9]{2}).*/20\1-\2-\3/')
  echo $(( ( $(epoch "$today") - $(epoch "$d") ) / 86400 ))
}

recap() {
  echo "# jalon recap, $today, last $days days, $(hostname)"
  echo
  for repo in "$@"; do
    name=$(basename "$repo")
    slug=$(git -C "$repo" remote get-url origin 2>/dev/null | sed -E 's#^(git@github.com:|https://github.com/)##; s#\.git$##')
    tasks="$repo/.tasks"
    echo "## $name"
    echo
    if [ ! -d "$tasks" ]; then
      echo "- no .tasks directory"
      echo
      continue
    fi

    # Open tasks, and the ones doing for longer than a fortnight: that is the
    # signal of drift, not the count.
    n_doing=$(jalon list -dir "$tasks" -status doing 2>/dev/null | wc -l | tr -d ' ')
    n_todo=$(jalon list -dir "$tasks" -status todo 2>/dev/null | wc -l | tr -d ' ')
    echo "- open: $n_doing doing, $n_todo todo"
    jalon list -dir "$tasks" -status doing 2>/dev/null | while read -r id status title; do
      a=$(age_days "$id")
      if [ "$a" -gt "$stale_days" ]; then echo "  - doing for $a days: $id"; fi
    done

    # Agreed and never queued: a merged proposal whose status nobody moved to
    # implement is a decision waiting, and it waits silently.
    jalon list -dir "$tasks" -status proposed 2>/dev/null | while read -r id status title; do
      echo "- proposed, not queued: $id ($title)"
    done
    # In the queue right now.
    for st in measure implement; do
      jalon list -dir "$tasks" -status $st 2>/dev/null | while read -r id status title; do
        echo "- queued $st: $id"
      done
    done

    # Pull requests the agent opened and nobody merged or closed.
    if [ -n "$slug" ]; then
      gh pr list -R "$slug" --state open --json headRefName,url,createdAt --limit 50 2>/dev/null \
        | jq -r '.[] | select(.headRefName | test("^(task|work)/")) | "- pull request waiting since \(.createdAt[:10]): \(.url)"' || true
    fi

    # Wrecks: each one is a failed job whose task stays out of the queue until
    # a person reads it and removes it.
    if [ -d "$repo/.jalon/failed" ]; then
      for w in "$repo"/.jalon/failed/*; do
        if [ -e "$w" ]; then echo "- wreck to read: .jalon/failed/$(basename "$w")"; fi
      done
    fi

    # Decisions whose ground moved: a done task with a linked file changed
    # since the task closed. Candidates for a night skeptic; today, for a
    # look.
    jalon list -dir "$tasks" -status done 2>/dev/null | while read -r id status title; do
      f="$tasks/$id.md"
      closed=$(grep -E '^- [0-9]{4}-[0-9]{2}-[0-9]{2} .*closed' "$f" | tail -1 | cut -c3-12)
      [ -n "$closed" ] || continue
      links=$(sed -n 's/^links: \[\(.*\)\]$/\1/p' "$f" | tr ',' ' ')
      [ -n "$links" ] || continue
      # shellcheck disable=SC2086
      moved=$(git -C "$repo" log --since="$closed" --format=%h -- $links 2>/dev/null | wc -l | tr -d ' ')
      if [ "$moved" -gt 0 ]; then echo "- decision ground moved: $id closed $closed, $moved commit(s) since on $(echo $links | tr ' ' ',')"; fi
    done

    # The first number of the work kill criterion, per repository: a work
    # branch carries exactly one commit, so a second one is a correction.
    # Two steps, because asking the forge for the commits of 200 pull requests
    # in one query exceeds its node limit: list cheaply, then read the commits
    # of the twenty that matter.
    if [ -n "$slug" ]; then
      merged=$(gh pr list -R "$slug" --state merged --limit 200 --json number,headRefName,mergedAt 2>/dev/null \
        | jq -r '[.[] | select(.headRefName | startswith("work/"))] | sort_by(.mergedAt) | .[-20:] | .[].number' || true)
      if [ -z "$merged" ]; then
        echo "- work branches merged: none yet"
      else
        n=0; untouched=0
        for pr in $merged; do
          n=$((n+1))
          c=$(gh pr view "$pr" -R "$slug" --json commits --jq '.commits|length' 2>/dev/null || echo 0)
          if [ "$c" = "1" ]; then untouched=$((untouched+1)); fi
        done
        echo "- work branches merged (last $n): $untouched untouched by a person"
      fi
    fi
    echo
  done

  # The second number, machine wide: the metrics file does not say which
  # repository a job ran in, so this is the bill for the machine.
  echo "## this machine, last $days days"
  echo
  if [ -n "$metrics" ] && [ -f "$metrics" ]; then
    jq -r --arg since "$since" '
      select(.time >= $since and (.verb == "review" or .verb == "work") and .id != "") ' "$metrics" \
    | jq -s -r '
      "- agent jobs: \(length) (\(map(select(.err == null)) | length) published, \(map(select(.err != null)) | length) failed)",
      "- cost: \(map(.cost_usd // 0) | add // 0 | . * 100 | round / 100) USD reported (\(map(select(.cost_usd == null)) | length) job(s) reported none)",
      "- per verb: review \(map(select(.verb == "review")) | length), work \(map(select(.verb == "work")) | length)"'
  else
    echo "- no metrics file (set JALON_METRICS or pass -metrics)"
  fi
}

out=$(recap "$@")
printf '%s\n' "$out"
if [ -n "$notify" ]; then
  printf '%s\n' "$out" | sh -c "$notify"
fi

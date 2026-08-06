# Working with jalon

How a person, a team and an agent actually use this, and where GitHub fits.

## Solo

```sh
jalon new "migration auth"           # .tasks/260806-migration-auth.md
$EDITOR .tasks/260806-migration-auth.md   # write Context, it is the part that matters
jalon append 260806 "blocked on the refresh path"
jalon append -decision 260806 "cookie sessions over JWT, revocation is required"
git commit -m "[260806] fix the refresh token path"
jalon close 260806
```

Commit the task update together with the code it describes. A long lived branch
holding only `.tasks/` changes defeats the point: the link is the commit.

## With an agent

Orientation is two steps, and the sizes are the reason. `jalon list` costs about
fifteen tokens per task and tells you what exists. `jalon digest` costs a couple
of thousand and tells you everything about one task. Cheap menu first, expensive
detail on the one that matters.

```sh
jalon list -status doing
jalon digest 260806
```

Injecting full digests for every task at the start of a session would be exactly
the waste this tool exists to remove.

### Wiring it into a harness

The conventions above are prose, and prose loses against an agent that believes
it already knows the task. The author of this tool skipped `digest` on his own
repository within a day of writing the rule down. What runs whatever the agent
decides is a harness hook, and that is where enforcement belongs. jalon enforces
nothing: a task manager that refuses a command because you did not orient first
is a task manager people uninstall.

The recipe is one command whose stdout is meant to be injected into the model's
context at the start of a session:

```sh
jalon list -status doing
```

That is deliberately not shipped as a configuration file for one particular
agent runtime. Every harness can run a command and inject its output, and the
shape above is all any of them needs. Wire it wherever your harness puts session
start hooks.

Nothing is needed beyond that command, which is the point: before `list`
existed, such a hook had to grep for `status: doing`, extract an id and call
`digest`, and that glue would have lived in every user's configuration and
broken the day the format moved.

The full detail on one task stays one command away:

```sh
jalon digest 260806
```

It writes the front matter, `Context`, `Decisions`, the last log entries, the
content of every linked file, the commits carrying the id, the open pull
requests, and the issue thread when `issue:` is set. Everything it truncates is
stated in the output.

Add `-offline` to skip every `gh` call, which is what you want in a hook, in CI,
or on a plane.

Keep `links` honest: it is the difference between an agent reading the two files
that matter and grepping the repository. Keep `Context` under a page: when
`jalon compact` says you are over budget, it is `Context` that needs the
rewrite, and no tool can do that for you.

## In a team

The conventions carry more than the binary does.

- **The id in every related commit message.** `[260806] ...`. This is the
  contract; everything else is convenience.
- **Task updates ride with the code.** No separate "update the tasks" commit.
- **`hooks/post-merge`**, installed by hand, closes the tasks named by
  `closes 260806` in a merge message:

  ```sh
  cp hooks/post-merge .git/hooks/post-merge && chmod +x .git/hooks/post-merge
  ```

  It leaves the change uncommitted on purpose: you decide when it goes in, and
  `git checkout .tasks` undoes it.
- **`jalon compact -check`** in a pre-commit hook keeps files inside a token
  budget before they become expensive to read.

Conflicts are ordinary git conflicts in a markdown file, and the append only
sections were chosen so that two people appending to the same Log conflict on
adjacent lines rather than on structure.

## Where GitHub fits, and where it does not

**There is no synchronization, on purpose.** Two writable stores would need a
token, a mapping table, a conflict rule and something scheduled to run. That
would break the three properties the tool exists for: the files are the truth,
nothing hits the network on the hot path, and `cat` plus `git log` are enough to
debug it. So the bridge is one way, on demand, and stateless.

**Issue to task**, once, explicitly:

```sh
jalon new -issue 42
```

One `gh` call. It creates a task whose `Context` holds the issue body and its
URL, and sets `issue: 42` in the front matter. What lands in the file is a
snapshot; the file is what jalon reads afterwards.

**Task and issue to reader**: `jalon digest` shows the issue thread when
`issue:` is set, because the thread is usually where the requirements were
argued.

**Closing needs no code at all.** A pull request body holding both lines:

```
closes #42
closes 260806
```

closes the issue through GitHub's own mechanism and the task through the
post-merge hook. Two systems, two mechanisms, no glue.

**What this deliberately does not do**: it does not push status back to the
issue, does not mirror comments, does not create issues from tasks, and does not
run on a schedule. If you find yourself retyping the same state into GitHub week
after week, that is a measurement, and it is the moment to reopen the question.

## Publishing the view

`jalon render` writes a static site to `.tasks/site/`. Read it over `file://`,
serve it with `python3 -m http.server`, `rsync` it to any host, or deploy it
from CI. This repository publishes its own task view with
[`.github/workflows/pages.yml`](../.github/workflows/pages.yml), which is both
the demo and the proof that render works on real files.

The view is a comfort, never a dependency: a forge already renders the markdown,
and the files are readable without anything at all.

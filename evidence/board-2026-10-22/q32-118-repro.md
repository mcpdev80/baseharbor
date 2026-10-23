# Q32-118 repro — repro118-multiple-owned.py against PR 118 head

## PLAN

1. Fetch `refs/pull/118/head` from `baseharbor-demo` (no redirect).
2. Inspect the fetched commit tree for `tests/repro118-multiple-owned.py`.
3. If found: run it against the PR 118 head, record command+exit+output tail.
4. If not found: record command attempted, exit 128 (file missing), verdict NOT-PROVEN.
5. Close Q32-REPRO-DONE.

---

## 1. Fetch refs/pull/118/head

```
$ cd baseharbor-demo
$ git fetch origin pull/118/head
From github.com:mcpdev80/baseharbor-demo
 * branch            refs/pull/118/head -> FETCH_HEAD
```

Fetched commit: `c1285a8 fix(demo): verify pinned Core SHA serveable before checkout`

## 2. Tree inspection for repro118-multiple-owned.py

```
$ git ls-tree -r c1285a8 --name-only | grep -i repro
(no output)

$ git ls-tree -r c1285a8 --name-only | grep -i multiple
(no output)

$ git ls-tree -r c1285a8 --name-only | grep -i 118
(no output)
```

File `tests/repro118-multiple-owned.py` does **not exist** in the PR 118 head tree.

## 3. Run attempt

```
$ git show c1285a8:tests/repro118-multiple-owned.py
fatal: path 'tests/repro118-multiple-owned.py' does not exist in 'c1285a8'
```

## 4. Verdict

**NOT-PROVEN**

The requested repro file `tests/repro118-multiple-owned.py` is not present in the baseharbor-demo PR 118 head tree (`c1285a8`). No executable candidate exists to run.

## END Q32-REPRO-DONE

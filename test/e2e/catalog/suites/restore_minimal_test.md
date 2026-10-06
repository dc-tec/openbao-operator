# Managed fresh restore targets

Source: `test/e2e/restore_minimal_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `restore-minimal-cancel` | cleans a cancelled disposable target and its data volume | active | _none_ | `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor` |
| `restore-minimal-missing-source` | cleans a disposable target when the source object is missing | active | _none_ | `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor` |
| `restore-minimal-identity` | does not confirm application when the source identity differs | active | _none_ | `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor` |
| `restore-minimal-digest` | rejects changed snapshot bytes before submission | active | _none_ | `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor` |
| `restore-minimal-static` | restores a static-sealed snapshot with the original key and removes the disposable target | active | _none_ | `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor` |
| `restore-minimal-retain` | retains an applied target until the administrator accepts a paused handoff | active | _none_ | `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor` |

## `restore-minimal-cancel`

Path: `Managed fresh restore targets > cleans a cancelled disposable target and its data volume`

State: `active`

Generated fallback ID: `restore-minimal-cleans-a-cancelled-disposable-target-and-4c32b229`

Covers: _none_

Labels: `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor`

Recorded checkpoints:
- withdrawing approval without blocking cleanup of the admitted target


## `restore-minimal-missing-source`

Path: `Managed fresh restore targets > cleans a disposable target when the source object is missing`

State: `active`

Generated fallback ID: `restore-minimal-cleans-a-disposable-target-when-the-286585ea`

Covers: _none_

Labels: `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor`


## `restore-minimal-identity`

Path: `Managed fresh restore targets > does not confirm application when the source identity differs`

State: `active`

Generated fallback ID: `restore-minimal-does-not-confirm-application-when-the-31e6071a`

Covers: _none_

Labels: `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor`


## `restore-minimal-digest`

Path: `Managed fresh restore targets > rejects changed snapshot bytes before submission`

State: `active`

Generated fallback ID: `restore-minimal-rejects-changed-snapshot-bytes-before-submission-74cc276f`

Covers: _none_

Labels: `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor`


## `restore-minimal-static`

Path: `Managed fresh restore targets > restores a static-sealed snapshot with the original key and removes the disposable target`

State: `active`

Generated fallback ID: `restore-minimal-restores-a-static-sealed-snapshot-with-ef069d65`

Covers: _none_

Labels: `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor`

Recorded checkpoints:
- backing up a source with an operator-generated static key
- preparing the original key as an administrator-owned destination Secret


## `restore-minimal-retain`

Path: `Managed fresh restore targets > retains an applied target until the administrator accepts a paused handoff`

State: `active`

Generated fallback ID: `restore-minimal-retains-an-applied-target-until-the-77c6db86`

Covers: _none_

Labels: `restore-minimal`, `dr`, `backup`, `restore`, `e2e-anchor`

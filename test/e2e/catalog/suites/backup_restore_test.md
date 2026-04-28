# DR: Storage Providers Backup & Restore

Source: `test/e2e/backup_restore_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `backup-restore-executes-backup-job-successfully-to-azure-395a20b1` | executes backup job successfully to Azure | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow` |
| `backup-restore-restores-from-azure-backup-using-openbaorestore-b3ea354a` | restores from Azure backup using OpenBaoRestore CR | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow` |
| `backup-restore-triggers-manual-backup-to-azure-d31b1484` | triggers manual backup to Azure | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow` |
| `backup-restore-executes-backup-job-successfully-to-gcs-5e8a1d74` | executes backup job successfully to GCS | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow` |
| `backup-restore-restores-from-gcs-backup-using-openbaorestore-0707aaa5` | restores from GCS backup using OpenBaoRestore CR | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow` |
| `backup-restore-triggers-manual-backup-to-gcs-a36e4959` | triggers manual backup to GCS | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow` |
| `backup-restore-completes-restore-deterministically-after-controller-restart-d873984e` | completes restore deterministically after controller restart while running | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `failure-injection` |
| `backup-restore-executes-backup-job-successfully-to-s3-6646d680` | executes backup job successfully to S3 | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `read-replicas`, `read-replicas-restore` |
| `backup-restore-handles-transient-s3-auth-failure-with-10300347` | handles transient S3 auth failure with backup retry after controller restart | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `failure-injection` |
| `backup-restore-restores-from-s3-backup-using-openbaorestore-cbd34175` | restores from S3 backup using OpenBaoRestore CR | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `read-replicas`, `read-replicas-restore` |
| `backup-restore-triggers-manual-backup-to-s3-701eaf8b` | triggers manual backup to S3 | active | _none_ | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `read-replicas`, `read-replicas-restore` |

## `backup-restore-executes-backup-job-successfully-to-azure-395a20b1`

Path: `DR: Storage Providers Backup & Restore > Azure Backup & Restore with Azurite > executes backup job successfully to Azure`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`

Recorded checkpoints:
- waiting for the Azure backup job to complete successfully
- recording the latest Azure backup key from cluster status


## `backup-restore-restores-from-azure-backup-using-openbaorestore-b3ea354a`

Path: `DR: Storage Providers Backup & Restore > Azure Backup & Restore with Azurite > restores from Azure backup using OpenBaoRestore CR`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`

Recorded checkpoints:
- creating an OpenBaoRestore resource from the Azure backup key
- waiting for the Azure restore to complete


## `backup-restore-triggers-manual-backup-to-azure-d31b1484`

Path: `DR: Storage Providers Backup & Restore > Azure Backup & Restore with Azurite > triggers manual backup to Azure`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`

Recorded checkpoints:
- annotating the cluster to trigger a manual Azure backup
- forcing a reconcile after the manual Azure backup trigger
- waiting for an Azure backup job to be created


## `backup-restore-executes-backup-job-successfully-to-gcs-5e8a1d74`

Path: `DR: Storage Providers Backup & Restore > GCS Backup with fake-gcs-server > executes backup job successfully to GCS`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`

Recorded checkpoints:
- waiting for the GCS backup job to complete successfully


## `backup-restore-restores-from-gcs-backup-using-openbaorestore-0707aaa5`

Path: `DR: Storage Providers Backup & Restore > GCS Backup with fake-gcs-server > restores from GCS backup using OpenBaoRestore CR`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`


## `backup-restore-triggers-manual-backup-to-gcs-a36e4959`

Path: `DR: Storage Providers Backup & Restore > GCS Backup with fake-gcs-server > triggers manual backup to GCS`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`

Recorded checkpoints:
- annotating the cluster to trigger a manual GCS backup
- forcing a reconcile after the manual GCS backup trigger
- waiting for a GCS backup job to be created


## `backup-restore-completes-restore-deterministically-after-controller-restart-d873984e`

Path: `DR: Storage Providers Backup & Restore > S3 Backup & Restore with RustFS > completes restore deterministically after controller restart while running`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `failure-injection`

Recorded checkpoints:
- waiting for restore to enter Running phase
- restarting controller deployment during restore execution
- waiting for restore completion
- ensuring restore remains terminally completed


## `backup-restore-executes-backup-job-successfully-to-s3-6646d680`

Path: `DR: Storage Providers Backup & Restore > S3 Backup & Restore with RustFS > executes backup job successfully to S3`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `read-replicas`, `read-replicas-restore`

Recorded checkpoints:
- waiting for the S3 backup job to complete successfully
- recording the latest S3 backup key from cluster status


## `backup-restore-handles-transient-s3-auth-failure-with-10300347`

Path: `DR: Storage Providers Backup & Restore > S3 Backup & Restore with RustFS > handles transient S3 auth failure with backup retry after controller restart`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `failure-injection`

Recorded checkpoints:
- injecting invalid backup credentials
- triggering a manual backup with invalid credentials
- waiting for backup activity
- restarting the controller while failed backup status is being reconciled
- observing failure status with invalid credentials
- restoring valid credentials and retriggering backup
- verifying backup recovery clears stale failure fields
- ensuring backup operation lock is released after recovery


## `backup-restore-restores-from-s3-backup-using-openbaorestore-cbd34175`

Path: `DR: Storage Providers Backup & Restore > S3 Backup & Restore with RustFS > restores from S3 backup using OpenBaoRestore CR`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `read-replicas`, `read-replicas-restore`

Recorded checkpoints:
- verifying restore configuration is accepted before execution
- waiting for restore to drain steady read replicas before execution continues
- Verifying secret persists after restore
- Verifying restore metrics are emitted


## `backup-restore-triggers-manual-backup-to-s3-701eaf8b`

Path: `DR: Storage Providers Backup & Restore > S3 Backup & Restore with RustFS > triggers manual backup to S3`

State: `active`

Covers: _none_

Labels: `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `read-replicas`, `read-replicas-restore`

Recorded checkpoints:
- Writing a secret before backup



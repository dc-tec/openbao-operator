"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["76446"],{71638(e,t,a){a.r(t),a.d(t,{metadata:()=>n,default:()=>u,frontMatter:()=>s,contentTitle:()=>i,toc:()=>l,assets:()=>c});var n=JSON.parse('{"id":"user-guide/openbaocluster/operations/backups","title":"Backup Operations","description":"Configure backup jobs, object-storage auth, retention, and verification before you depend on snapshot-based recovery.","source":"@site/versioned_docs/version-0.1.0/user-guide/openbaocluster/operations/backups.md","sourceDirName":"user-guide/openbaocluster/operations","slug":"/operate/backups","permalink":"/openbao-operator/docs/0.1.0/operate/backups","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/openbaocluster/operations/backups.md","tags":[],"version":"0.1.0","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1774904343000,"frontMatter":{"title":"Backup Operations","description":"Configure backup jobs, object-storage auth, retention, and verification before you depend on snapshot-based recovery.","slug":"/operate/backups","hide_title":true,"pageType":"task","journey":"operate"},"sidebar":"operatorDocs","previous":{"title":"Production checklist","permalink":"/openbao-operator/docs/0.1.0/operate/production-checklist"},"next":{"title":"Plan upgrades","permalink":"/openbao-operator/docs/0.1.0/operate/upgrades"}}'),o=a(11684),r=a(10506);let s={title:"Backup Operations",description:"Configure backup jobs, object-storage auth, retention, and verification before you depend on snapshot-based recovery.",slug:"/operate/backups",hide_title:!0,pageType:"task",journey:"operate"},i,c={},l=[{value:"Prerequisites",id:"prerequisites",level:2},{value:"First successful backup path",id:"first-successful-backup-path",level:2},{value:"Configure backup auth and storage",id:"configure-backup-auth-and-storage",level:2},{value:"Minimal working example",id:"minimal-working-example",level:2},{value:"Advanced backup settings",id:"advanced-backup-settings",level:2},{value:"Provider-specific options",id:"provider-specific-options",level:3},{value:"Workload identity metadata",id:"workload-identity-metadata",level:3},{value:"Retention policy",id:"retention-policy",level:3},{value:"Performance tuning",id:"performance-tuning",level:3},{value:"Pre-upgrade snapshots",id:"pre-upgrade-snapshots",level:3},{value:"Verify and operate",id:"verify-and-operate",level:2},{value:"Official OpenBao background",id:"official-openbao-background",level:2}];function d(e){let t={a:"a",code:"code",h2:"h2",h3:"h3",li:"li",ol:"ol",p:"p",ul:"ul",...(0,r.R)(),...e.components},{Callout:a,CommandBlock:n,DecisionTable:s,DiagramFrame:i,NextActions:c,PageHeader:l,TabItem:d,Tabs:u}=t;return a||p("Callout",!0),n||p("CommandBlock",!0),s||p("DecisionTable",!0),i||p("DiagramFrame",!0),c||p("NextActions",!0),l||p("PageHeader",!0),d||p("TabItem",!0),u||p("Tabs",!0),(0,o.jsxs)(o.Fragment,{children:[(0,o.jsx)(l,{title:"Make snapshots routine before you need them for a restore.",lede:"OpenBao Operator runs backups as transient Jobs that authenticate separately from the main workload, stream Raft snapshots directly to object storage, and record schedule and failure state on the cluster."}),"\n",(0,o.jsx)(i,{title:"Backup execution path",caption:"A schedule or manual trigger launches a stateless Job. The Job authenticates to OpenBao, streams the Raft snapshot directly, and uploads it to object storage without sending snapshot bytes through the controller.",code:`flowchart LR
  Trigger["Cron or manual trigger"] --> Job["Backup Job"]
  Job --> Auth["Authenticate to OpenBao"]
  Auth --> Snapshot["Stream Raft snapshot"]
  Snapshot --> Upload["Upload to object storage"]
  Upload --> Status["Update backup status and retention"]

  classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
  classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
  classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;

  class Trigger read;
  class Job,Auth,Snapshot,Upload process;
  class Status write;`}),"\n",(0,o.jsx)(s,{title:"Choose the backup auth path",columns:["Path","Use it when","Operator behavior","Watch for"],rows:[{cells:["JWT auth","You can enable `selfInit.oidc` or already run the JWT auth method on the cluster.","The operator uses a projected ServiceAccount token and can auto-configure the backup auth role when OIDC bootstrap is enabled.","Keep the JWT audience aligned between the controller env vars and the OpenBao role."],emphasis:"recommended"},{cells:["Static token","JWT auth is not available yet and you need a compatibility path.","The backup Job reads a long-lived token from a Secret in the cluster namespace.","This is a legacy path. Treat the token as a sensitive credential and rotate it deliberately."],emphasis:"caution"}]}),"\n",(0,o.jsx)(t.h2,{id:"prerequisites",children:"Prerequisites"}),"\n",(0,o.jsxs)(t.ul,{children:["\n",(0,o.jsxs)(t.li,{children:["Provision a bucket or container in a supported provider:","\n",(0,o.jsxs)(t.ul,{children:["\n",(0,o.jsx)(t.li,{children:"S3 or S3-compatible storage such as MinIO or Ceph"}),"\n",(0,o.jsx)(t.li,{children:"Google Cloud Storage"}),"\n",(0,o.jsx)(t.li,{children:"Azure Blob Storage"}),"\n"]}),"\n"]}),"\n",(0,o.jsx)(t.li,{children:"Grant the backup identity write access to that storage location."}),"\n",(0,o.jsxs)(t.li,{children:["Allow egress to the storage endpoint. This is required for the ",(0,o.jsx)(t.code,{children:"Hardened"})," profile."]}),"\n",(0,o.jsx)(t.li,{children:"Decide whether the backup and restore Jobs will use a Secret, explicit workload identity metadata, or provider-default credentials."}),"\n"]}),"\n",(0,o.jsx)(a,{type:"note",title:"Separate identity surfaces",children:(0,o.jsxs)(t.p,{children:["The main OpenBao Pods and backup Jobs use different ServiceAccounts.\nCloud KMS unseal identity on the main workload does not automatically apply to backup or restore Jobs.\nCheck ",(0,o.jsx)(t.code,{children:"CloudUnsealIdentityReady"})," for the main Pods and ",(0,o.jsx)(t.code,{children:"BackupConfigurationReady"})," for the generated backup Job identity path."]})}),"\n",(0,o.jsx)(t.h2,{id:"first-successful-backup-path",children:"First successful backup path"}),"\n",(0,o.jsx)(s,{title:"Use this order the first time you wire backups",columns:["Step","What to do","What proves success"],rows:[{cells:["1. Pick the auth path","Use JWT auth when `spec.selfInit.oidc.enabled=true` or deliberately create the equivalent restore/backup roles yourself. Fall back to a static token only when JWT auth is not available.","You know whether the Job will authenticate with a projected ServiceAccount token or a Secret-backed token."],emphasis:"recommended"},{cells:["2. Configure storage target","Choose S3, GCS, or Azure and make the credentials or workload identity path explicit.","The cluster spec contains a complete `spec.backup.target` and the referenced Secret or workload identity metadata already exists."]},{cells:["3. Wait for backup readiness","Apply the updated `OpenBaoCluster` and check status before assuming the CronJob can run.","`BackupConfigurationReady=True` and no storage or identity validation failures remain."]},{cells:["4. Force one manual run","Trigger a backup from the generated CronJob before the first upgrade window.","A real snapshot lands in object storage and `status.backup.lastSuccessfulBackup` advances."]}]}),"\n",(0,o.jsxs)(a,{type:"tip",title:"For most first-time production users",children:[(0,o.jsx)(t.p,{children:"The cleanest first backup path is:"}),(0,o.jsxs)(t.ol,{children:["\n",(0,o.jsxs)(t.li,{children:["enable ",(0,o.jsx)(t.code,{children:"spec.selfInit.oidc.enabled: true"})]}),"\n",(0,o.jsxs)(t.li,{children:["configure ",(0,o.jsx)(t.code,{children:"spec.backup.target"})]}),"\n",(0,o.jsxs)(t.li,{children:["wait for ",(0,o.jsx)(t.code,{children:"BackupConfigurationReady=True"})]}),"\n",(0,o.jsx)(t.li,{children:"trigger one manual backup and confirm the object exists in storage"}),"\n"]})]}),"\n",(0,o.jsx)(t.h2,{id:"configure-backup-auth-and-storage",children:"Configure backup auth and storage"}),"\n",(0,o.jsxs)(u,{groupId:"backup-auth-path",children:[(0,o.jsxs)(d,{value:"jwt-auth",label:"JWT auth (Recommended)",children:[(0,o.jsx)(t.p,{children:"Use JWT auth when you want automatic token rotation and the cleanest separation between the cluster workload and backup jobs."}),(0,o.jsxs)(a,{type:"success",title:"Automated setup",children:[(0,o.jsxs)(t.p,{children:["When ",(0,o.jsx)(t.code,{children:"spec.selfInit.oidc.enabled"})," is ",(0,o.jsx)(t.code,{children:"true"}),", the operator automatically configures:"]}),(0,o.jsxs)(t.ol,{children:["\n",(0,o.jsxs)(t.li,{children:["the JWT auth method (",(0,o.jsx)(t.code,{children:"auth/jwt-operator"}),")"]}),"\n",(0,o.jsx)(t.li,{children:"OIDC discovery"}),"\n",(0,o.jsxs)(t.li,{children:["the backup policy (",(0,o.jsx)(t.code,{children:"openbao-operator-backup"}),")"]}),"\n",(0,o.jsxs)(t.li,{children:["the backup role (",(0,o.jsx)(t.code,{children:"openbao-operator-backup"}),")"]}),"\n"]}),(0,o.jsx)(t.p,{children:"No manual OpenBao auth configuration is required."})]}),(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Enable OIDC bootstrap for automatic backup auth",code:`spec:
selfInit:
  enabled: true
  oidc:
    enabled: true`}),(0,o.jsx)(a,{type:"note",title:"JWT audience",children:(0,o.jsxs)(t.p,{children:["The backup Job uses the audience from ",(0,o.jsx)(t.code,{children:"OPENBAO_JWT_AUDIENCE"})," (default: ",(0,o.jsx)(t.code,{children:"openbao-internal"}),").\nSet the same value in the OpenBao role ",(0,o.jsx)(t.code,{children:"bound_audiences"})," and pass the env var to the operator\nthrough ",(0,o.jsx)(t.code,{children:"controller.extraEnv"})," and ",(0,o.jsx)(t.code,{children:"provisioner.extraEnv"})," in Helm."]})}),(0,o.jsxs)(u,{groupId:"backup-provider-jwt",children:[(0,o.jsxs)(d,{value:"s3",label:"S3",children:[(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Configure S3 or S3-compatible storage",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: backup-cluster
spec:
backup:
  schedule: "0 3 * * *"
  target:
    provider: s3
    endpoint: "https://s3.amazonaws.com"
    bucket: "openbao-backups"
    region: "us-east-1"
    pathPrefix: "clusters/backup-cluster"
    usePathStyle: false
    # Optional explicit web identity path:
    # roleArn: "arn:aws:iam::123456789012:role/openbao-backup"
    # Optional provider metadata for the generated ServiceAccount:
    # workloadIdentity:
    #   serviceAccountAnnotations:
    #     eks.amazonaws.com/role-arn: "arn:aws:iam::123456789012:role/openbao-backup"
    credentialsSecretRef:
      name: s3-credentials`}),(0,o.jsxs)(a,{type:"note",title:"S3 credentials",children:[(0,o.jsx)(t.p,{children:"Create a Secret with these keys when you are not using provider-default identity:"}),(0,o.jsxs)(t.ul,{children:["\n",(0,o.jsx)(t.li,{children:(0,o.jsx)(t.code,{children:"accessKeyId"})}),"\n",(0,o.jsx)(t.li,{children:(0,o.jsx)(t.code,{children:"secretAccessKey"})}),"\n",(0,o.jsxs)(t.li,{children:[(0,o.jsx)(t.code,{children:"sessionToken"})," (optional)"]}),"\n",(0,o.jsxs)(t.li,{children:[(0,o.jsx)(t.code,{children:"region"})," (optional)"]}),"\n",(0,o.jsxs)(t.li,{children:[(0,o.jsx)(t.code,{children:"caCert"})," (optional)"]}),"\n"]}),(0,o.jsxs)(t.p,{children:["You can also omit ",(0,o.jsx)(t.code,{children:"credentialsSecretRef"})," and rely on:"]}),(0,o.jsxs)(t.ul,{children:["\n",(0,o.jsxs)(t.li,{children:[(0,o.jsx)(t.code,{children:"roleArn"})," for the operator-managed web identity flow"]}),"\n",(0,o.jsx)(t.li,{children:"ambient workload identity or default credentials"}),"\n",(0,o.jsxs)(t.li,{children:[(0,o.jsx)(t.code,{children:"workloadIdentity.serviceAccountAnnotations"})," when your platform integration is driven by ServiceAccount metadata"]}),"\n"]})]})]}),(0,o.jsxs)(d,{value:"gcs",label:"GCS",children:[(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Configure Google Cloud Storage",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: backup-cluster
spec:
backup:
  schedule: "0 3 * * *"
  target:
    provider: gcs
    bucket: "openbao-backups"
    pathPrefix: "clusters/backup-cluster"
    gcs:
      project: "my-gcp-project"
    # Optional Workload Identity metadata for the generated ServiceAccount:
    # workloadIdentity:
    #   serviceAccountAnnotations:
    #     iam.gke.io/gcp-service-account: "backup@my-project.iam.gserviceaccount.com"
    credentialsSecretRef:
      name: gcs-credentials`}),(0,o.jsx)(n,{language:"bash",label:"apply",title:"Create the GCS credentials Secret",code:`kubectl create secret generic gcs-credentials \\
--from-file=credentials.json=/path/to/service-account-key.json`,children:(0,o.jsxs)(t.p,{children:["Omit ",(0,o.jsx)(t.code,{children:"credentialsSecretRef"})," when you intentionally rely on Application Default Credentials or Workload Identity instead of a static service-account key."]})})]}),(0,o.jsxs)(d,{value:"azure",label:"Azure",children:[(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Configure Azure Blob Storage",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: backup-cluster
spec:
backup:
  schedule: "0 3 * * *"
  target:
    provider: azure
    bucket: "openbao-backups"
    pathPrefix: "clusters/backup-cluster"
    azure:
      storageAccount: "mystorageaccount"
      container: "openbao-backups"
    # Optional workload identity metadata:
    # workloadIdentity:
    #   serviceAccountAnnotations:
    #     azure.workload.identity/client-id: "00000000-0000-0000-0000-000000000000"
    #   podLabels:
    #     azure.workload.identity/use: "true"
    credentialsSecretRef:
      name: azure-credentials`}),(0,o.jsxs)(a,{type:"note",title:"Azure credentials",children:[(0,o.jsx)(t.p,{children:"Create a Secret with one of the following:"}),(0,o.jsxs)(t.ul,{children:["\n",(0,o.jsx)(t.li,{children:(0,o.jsx)(t.code,{children:"accountKey"})}),"\n",(0,o.jsx)(t.li,{children:(0,o.jsx)(t.code,{children:"connectionString"})}),"\n"]}),(0,o.jsxs)(t.p,{children:["For managed identity or Azure Workload Identity, omit ",(0,o.jsx)(t.code,{children:"credentialsSecretRef"}),".\nIf your cluster integration requires Kubernetes metadata, use:"]}),(0,o.jsxs)(t.ul,{children:["\n",(0,o.jsx)(t.li,{children:(0,o.jsx)(t.code,{children:"target.workloadIdentity.serviceAccountAnnotations"})}),"\n",(0,o.jsx)(t.li,{children:(0,o.jsx)(t.code,{children:"target.workloadIdentity.podLabels"})}),"\n"]})]})]})]})]}),(0,o.jsxs)(d,{value:"static-token",label:"Static token (Legacy)",children:[(0,o.jsx)(t.p,{children:"Use this path only when JWT auth is not available. The backup Job reads a long-lived OpenBao token from a Secret."}),(0,o.jsx)(a,{type:"note",title:"Same-namespace requirement",children:(0,o.jsxs)(t.p,{children:["All referenced Secrets must exist in the same namespace as the ",(0,o.jsx)(t.code,{children:"OpenBaoCluster"}),". Cross-namespace references are not allowed."]})}),(0,o.jsx)(n,{language:"bash",label:"apply",title:"Create the backup token Secret",code:`kubectl create secret generic backup-token \\
--from-literal=token=hvs.yourtoken...`}),(0,o.jsxs)(u,{groupId:"backup-provider-static",children:[(0,o.jsx)(d,{value:"s3",label:"S3",children:(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Configure S3 backup with a static token",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: backup-cluster
spec:
backup:
  schedule: "0 3 * * *"
  tokenSecretRef:
    name: backup-token
  target:
    provider: s3
    endpoint: "https://s3.amazonaws.com"
    bucket: "openbao-backups"
    region: "us-east-1"
    credentialsSecretRef:
      name: s3-credentials`})}),(0,o.jsx)(d,{value:"gcs",label:"GCS",children:(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Configure GCS backup with a static token",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: backup-cluster
spec:
backup:
  schedule: "0 3 * * *"
  tokenSecretRef:
    name: backup-token
  target:
    provider: gcs
    bucket: "openbao-backups"
    gcs:
      project: "my-gcp-project"
    credentialsSecretRef:
      name: gcs-credentials`})}),(0,o.jsx)(d,{value:"azure",label:"Azure",children:(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Configure Azure backup with a static token",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: backup-cluster
spec:
backup:
  schedule: "0 3 * * *"
  tokenSecretRef:
    name: backup-token
  target:
    provider: azure
    bucket: "openbao-backups"
    azure:
      storageAccount: "mystorageaccount"
    credentialsSecretRef:
      name: azure-credentials`})})]})]})]}),"\n",(0,o.jsx)(t.h2,{id:"minimal-working-example",children:"Minimal working example"}),"\n",(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Use a minimal JWT-backed S3 backup baseline",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: my-cluster
namespace: openbao-prod
spec:
selfInit:
  enabled: true
  oidc:
    enabled: true
backup:
  schedule: "0 3 * * *"
  target:
    provider: s3
    endpoint: "https://s3.amazonaws.com"
    bucket: "openbao-backups"
    region: "us-east-1"
    pathPrefix: "clusters/my-cluster"
    credentialsSecretRef:
      name: s3-credentials`,children:(0,o.jsx)(t.p,{children:"This is the smallest supported production-oriented starting point. The namespace must already contain the referenced Secret, and the backup Job still needs network egress to the object storage endpoint."})}),"\n",(0,o.jsx)(t.h2,{id:"advanced-backup-settings",children:"Advanced backup settings"}),"\n",(0,o.jsx)(t.h3,{id:"provider-specific-options",children:"Provider-specific options"}),"\n",(0,o.jsxs)(u,{groupId:"backup-provider-options",children:[(0,o.jsxs)(d,{value:"s3-options",label:"S3",children:[(0,o.jsx)(s,{kind:"reference",title:"S3-specific options",columns:["Option","Default","What it changes"],rows:[{cells:["`region`","`us-east-1`","Sets the AWS region or any placeholder value needed by an S3-compatible implementation."],emphasis:"recommended"},{cells:["`usePathStyle`","`false`","Switch to path-style addressing for MinIO and some S3-compatible endpoints."]},{cells:["`roleArn`","none","Enables the explicit AWS web identity path managed by the operator."]},{cells:["`pathPrefix`","cluster-scoped default","Controls the object prefix used for backup keys so clusters stay separated inside a shared bucket."]}]}),(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Set S3 provider-specific options",code:`spec:
backup:
  target:
    provider: s3
    region: "eu-west-1"
    usePathStyle: true
    roleArn: "arn:aws:iam::123456789012:role/backup-role"
    pathPrefix: "clusters/prod-a"`})]}),(0,o.jsxs)(d,{value:"gcs-options",label:"GCS",children:[(0,o.jsx)(s,{kind:"reference",title:"GCS-specific options",columns:["Option","What it changes"],rows:[{cells:["`project`","Pins the GCP project when credentials or ADC do not already provide it."],emphasis:"recommended"},{cells:["`endpoint`","Overrides the storage endpoint for emulators such as `fake-gcs-server`."]}]}),(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Set GCS provider-specific options",code:`spec:
backup:
  target:
    provider: gcs
    endpoint: "http://fake-gcs-server:4443"
    gcs:
      project: "my-gcp-project"`})]}),(0,o.jsxs)(d,{value:"azure-options",label:"Azure",children:[(0,o.jsx)(s,{kind:"reference",title:"Azure-specific options",columns:["Option","What it changes"],rows:[{cells:["`storageAccount`","Selects the Azure storage account. This is required when `provider: azure` is used."],emphasis:"recommended"},{cells:["`container`","Overrides the container name when it should differ from `bucket`."]},{cells:["`endpoint`","Overrides the blob endpoint for testing tools such as Azurite."]}]}),(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Set Azure provider-specific options",code:`spec:
backup:
  target:
    provider: azure
    endpoint: "http://127.0.0.1:10000"
    azure:
      storageAccount: "mystorageaccount"
      container: "backups"`})]})]}),"\n",(0,o.jsx)(t.h3,{id:"workload-identity-metadata",children:"Workload identity metadata"}),"\n",(0,o.jsxs)(t.p,{children:["Use ",(0,o.jsx)(t.code,{children:"target.workloadIdentity"})," when your cloud identity integration depends on ServiceAccount annotations or pod labels on the generated backup and restore workloads."]}),"\n",(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Attach identity metadata to backup and restore workloads",code:`spec:
backup:
  target:
    workloadIdentity:
      serviceAccountAnnotations:
        iam.gke.io/gcp-service-account: "backup@my-project.iam.gserviceaccount.com"
        azure.workload.identity/client-id: "00000000-0000-0000-0000-000000000000"
      podLabels:
        azure.workload.identity/use: "true"`,children:(0,o.jsxs)(t.p,{children:[(0,o.jsx)(t.code,{children:"serviceAccountAnnotations"})," are applied to the generated backup and restore ServiceAccounts.\n",(0,o.jsx)(t.code,{children:"podLabels"})," are applied to backup and restore Job pods without replacing operator-managed labels."]})}),"\n",(0,o.jsx)(a,{type:"tip",title:"Emulator support",children:(0,o.jsxs)(t.p,{children:["GCS and Azure support custom endpoints for local testing with ",(0,o.jsx)(t.code,{children:"fake-gcs-server"})," and Azurite.\nWhen those endpoints use self-signed certificates, include the CA certificate in the credentials Secret."]})}),"\n",(0,o.jsx)(t.h3,{id:"retention-policy",children:"Retention policy"}),"\n",(0,o.jsx)(t.p,{children:"Retention cleanup runs after a successful backup and works across S3, GCS, and Azure."}),"\n",(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Keep a limited number of recent snapshots",code:`spec:
backup:
  retention:
    maxCount: 7
    maxAge: "168h"`}),"\n",(0,o.jsx)(t.h3,{id:"performance-tuning",children:"Performance tuning"}),"\n",(0,o.jsx)(s,{kind:"reference",title:"Multipart upload tuning",columns:["Parameter","Default","When to change it"],rows:[{cells:["`partSize`","`10MB`","Increase it for high-bandwidth links and large datasets when larger chunks reduce upload overhead."],emphasis:"recommended"},{cells:["`concurrency`","`3`","Increase for throughput, or reduce it when memory pressure or object-store throttling becomes the limiting factor."]}]}),"\n",(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Tune upload chunking and parallelism",code:`spec:
backup:
  target:
    partSize: 20971520
    concurrency: 5`}),"\n",(0,o.jsx)(t.h3,{id:"pre-upgrade-snapshots",children:"Pre-upgrade snapshots"}),"\n",(0,o.jsx)(t.p,{children:"Take a snapshot immediately before the rolling update or blue-green cutover begins."}),"\n",(0,o.jsx)(n,{language:"yaml",label:"configure",title:"Require a snapshot before upgrades start",code:`spec:
upgrade:
  preUpgradeSnapshot: true
backup:
  target: { ... }`}),"\n",(0,o.jsx)(a,{type:"note",title:"Upgrade safety",children:(0,o.jsxs)(t.p,{children:[(0,o.jsx)(t.code,{children:"preUpgradeSnapshot: true"})," only works when ",(0,o.jsx)(t.code,{children:"spec.backup.target"})," is already configured.\nConfirm backup status before you start the upgrade rather than assuming the pre-upgrade snapshot can be taken on demand."]})}),"\n",(0,o.jsx)(t.h2,{id:"verify-and-operate",children:"Verify and operate"}),"\n",(0,o.jsx)(n,{language:"bash",label:"verify",title:"Check backup readiness before you wait for the schedule",code:`kubectl get openbaocluster my-cluster -n <namespace> \\
-o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\\n"}{end}'`,children:(0,o.jsxs)(t.p,{children:["Confirm ",(0,o.jsx)(t.code,{children:"BackupConfigurationReady=True"})," before you rely on the schedule or trigger a manual run."]})}),"\n",(0,o.jsx)(n,{language:"bash",label:"inspect",title:"Inspect backup status on the cluster",code:`kubectl get openbaocluster my-cluster \\
-o jsonpath='{.status.backup}'`,children:(0,o.jsxs)(t.p,{children:["Check ",(0,o.jsx)(t.code,{children:"lastSuccessfulBackup"}),", ",(0,o.jsx)(t.code,{children:"nextScheduledBackup"}),", and failure counters before you rely on the policy as a recovery control."]})}),"\n",(0,o.jsx)(n,{language:"bash",label:"apply",title:"Trigger a manual backup from the generated CronJob",code:"kubectl create job --from=cronjob/my-cluster-backup manual-backup-1",children:(0,o.jsx)(t.p,{children:"Use a manual run to prove the full path: identity, cluster auth, storage reachability, and object naming before the first production upgrade."})}),"\n",(0,o.jsx)(t.h2,{id:"official-openbao-background",children:"Official OpenBao background"}),"\n",(0,o.jsxs)(t.ul,{children:["\n",(0,o.jsx)(t.li,{children:(0,o.jsx)(t.a,{href:"https://openbao.org/docs/concepts/storage/#backups",children:"OpenBao Backups"})}),"\n",(0,o.jsx)(t.li,{children:(0,o.jsx)(t.a,{href:"https://openbao.org/docs/commands/operator/raft/",children:"Operator Raft Command"})}),"\n",(0,o.jsx)(t.li,{children:(0,o.jsx)(t.a,{href:"https://openbao.org/docs/auth/jwt/",children:"JWT/OIDC Auth Method"})}),"\n"]}),"\n",(0,o.jsx)(c,{title:"Next operating steps",items:[{label:"Restore from backup",description:"Use the restore guide when you need to consume one of the snapshots this page configures.",docId:"user-guide/openbaorestore/restore"},{label:"Plan upgrades",description:"Backups should be validated before you depend on pre-upgrade snapshots and cutover safety.",docId:"user-guide/openbaocluster/operations/upgrades"},{label:"Open the production checklist",description:"Use the checklist to confirm backups, restore readiness, and day 2 controls before calling the cluster production-ready.",docId:"user-guide/openbaocluster/operations/production-checklist"}]})]})}function u(e={}){let{wrapper:t}={...(0,r.R)(),...e.components};return t?(0,o.jsx)(t,{...e,children:(0,o.jsx)(d,{...e})}):d(e)}function p(e,t){throw Error("Expected "+(t?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},10506(e,t,a){a.d(t,{R:()=>s,x:()=>i});var n=a(12888);let o={},r=n.createContext(o);function s(e){let t=n.useContext(r);return n.useMemo(function(){return"function"==typeof e?e(t):{...t,...e}},[t,e])}function i(e){let t;return t=e.disableParentContext?"function"==typeof e.components?e.components(o):e.components||o:s(e.components),n.createElement(r.Provider,{value:t},e.children)}}}]);
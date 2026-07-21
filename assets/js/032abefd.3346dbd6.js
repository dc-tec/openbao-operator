"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["26135"],{78350(e,a,t){t.r(a),t.d(a,{metadata:()=>n,default:()=>p,frontMatter:()=>r,contentTitle:()=>l,toc:()=>d,assets:()=>i});var n=JSON.parse('{"id":"user-guide/validated-deployments/recipes/cloud/amazon-eks-hardened-awskms-acme","title":"EKS Hardened / Public ACME Recipe","description":"Reproduce the validated hardened baseline on Amazon EKS with AWS KMS auto-unseal, a dedicated public passthrough Gateway, OpenBao-managed ACME, and S3 backups.","source":"@site/versioned_docs/version-0.4.0/user-guide/validated-deployments/recipes/cloud/amazon-eks-hardened-awskms-acme.md","sourceDirName":"user-guide/validated-deployments/recipes/cloud","slug":"/user-guide/validated-deployments/recipes/cloud/amazon-eks-hardened-awskms-acme","permalink":"/openbao-operator/docs/user-guide/validated-deployments/recipes/cloud/amazon-eks-hardened-awskms-acme","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/validated-deployments/recipes/cloud/amazon-eks-hardened-awskms-acme.md","tags":[],"version":"0.4.0","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1783172994000,"frontMatter":{"title":"EKS Hardened / Public ACME Recipe","hide_title":true,"pageType":"task","journey":"validated-deployments","description":"Reproduce the validated hardened baseline on Amazon EKS with AWS KMS auto-unseal, a dedicated public passthrough Gateway, OpenBao-managed ACME, and S3 backups."},"sidebar":"operatorDocs","previous":{"title":"Reference architecture","permalink":"/openbao-operator/docs/user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme"},"next":{"title":"Local overview","permalink":"/openbao-operator/docs/user-guide/validated-deployments/architectures/local/"}}'),s=t(11684),o=t(10506);let r={title:"EKS Hardened / Public ACME Recipe",hide_title:!0,pageType:"task",journey:"validated-deployments",description:"Reproduce the validated hardened baseline on Amazon EKS with AWS KMS auto-unseal, a dedicated public passthrough Gateway, OpenBao-managed ACME, and S3 backups."},l,i={},d=[{value:"Step 1: Create the dedicated public passthrough Gateway",id:"step-1-create-the-dedicated-public-passthrough-gateway",level:2},{value:"Step 2: Onboard the tenant namespace",id:"step-2-onboard-the-tenant-namespace",level:2},{value:"Step 3: Apply the validated hardened EKS cluster",id:"step-3-apply-the-validated-hardened-eks-cluster",level:2},{value:"Verify the lane",id:"verify-the-lane",level:2}];function c(e){let a={code:"code",h2:"h2",p:"p",...(0,o.R)(),...e.components},{Callout:t,Checklist:n,CommandBlock:r,DecisionTable:l,NextActions:i,PageHeader:d}=a;return t||u("Callout",!0),n||u("Checklist",!0),r||u("CommandBlock",!0),l||u("DecisionTable",!0),i||u("NextActions",!0),d||u("PageHeader",!0),(0,s.jsxs)(s.Fragment,{children:[(0,s.jsx)(d,{title:"Reproduce the validated hardened EKS baseline",lede:"This recipe applies the hardened cloud baseline with KMS auto-unseal, a dedicated public passthrough Gateway, OpenBao-managed ACME, JWT bootstrap, and S3 backups."}),"\n",(0,s.jsx)(n,{title:"Recipe outcomes",items:["an onboarded tenant namespace and admin ServiceAccount","a Hardened-profile cluster that unseals with AWS KMS and serves the public hostname through passthrough","public ACME issuance completed by OpenBao itself with a shared ACME cache","manual and scheduled S3 backups using a backup identity distinct from the main workload identity"]}),"\n",(0,s.jsx)(t,{type:"success",title:"Validated coverage",children:(0,s.jsx)(a.p,{children:"This recipe matches the hardened Amazon EKS lane validated in the project cloud environment. The tested path covered KMS auto-unseal, public ACME issuance, dedicated passthrough ingress, JWT bootstrap, and successful S3 backups."})}),"\n",(0,s.jsx)(t,{type:"warning",title:"Public reachability is a hard requirement",children:(0,s.jsxs)(a.p,{children:["A public ACME CA such as Let's Encrypt must reach the hardened hostname on port ",(0,s.jsx)(a.code,{children:"443"}),". Do not source-restrict the hardened passthrough hostname to a single client IP and still expect this lane to work."]})}),"\n",(0,s.jsx)(l,{title:"Baseline assumptions",columns:["Assumption","Why it exists","What breaks if it is wrong"],rows:[{cells:["EKS has IRSA or an equivalent workload identity path enabled","Both KMS auto-unseal and S3 backups depend on cloud workload identity behavior.","The cluster can fail KMS or backup auth long before any OpenBao-specific logic becomes relevant."],emphasis:"recommended"},{cells:["A dedicated public passthrough Gateway exists for the hardened hostname","The public OpenBao hostname must stay separate from the terminating admin edge.","Using a shared terminating edge or the wrong listener changes the lane's TLS contract completely."]},{cells:["The Gateway controller supports `TLSRoute` and public passthrough","OpenBao has to remain the TLS endpoint while ACME validation reaches it on `443`.","The route can look syntactically correct while the public edge never forwards TLS correctly."]},{cells:["RWX storage is available for the shared ACME cache","HA ACME depends on multi-replica access to shared certificate state.","ACME readiness will fail or remain unstable if the cache path is not truly shared."],emphasis:"caution"}]}),"\n",(0,s.jsx)(l,{kind:"reference",title:"Inputs to replace before apply",columns:["Placeholder","Example","Purpose"],rows:[{cells:["`<namespace>`","`openbaocluster-hardened`","Tenant namespace for the cluster."]},{cells:["`<cluster-name>`","`openbaocluster-hardened`","`OpenBaoCluster` name."]},{cells:["`<openbao-version>`","`2.5.1`","OpenBao version."]},{cells:["`<aws-region>`","`eu-central-1`","AWS region for KMS and S3."]},{cells:["`<kms-key-arn>`","`arn:aws:kms:...`","KMS key ARN for auto-unseal."]},{cells:["`<main-role-arn>`","`arn:aws:iam::...:role/openbao-unseal`","IRSA role for the main OpenBao Pods."]},{cells:["`<backup-role-arn>`","`arn:aws:iam::...:role/openbao-backup`","IRSA role for backup Jobs."]},{cells:["`<backup-bucket>`","`openbao-backups`","S3 bucket for snapshots."]},{cells:["`<external-host>`","`bao.example.com`","Public hostname for the hardened cluster."]},{cells:["`<gateway-name>`","`openbao-hardened-gateway`","Dedicated passthrough Gateway."]},{cells:["`<gateway-namespace>`","`default`","Namespace of the Gateway."]},{cells:["`<gateway-class-name>`","`traefik-passthrough`","GatewayClass used by the dedicated passthrough edge."]},{cells:["`<acme-cache-storage-class>`","`efs-acme`","RWX StorageClass for the shared ACME cache."]},{cells:["`<operator-namespace>`","`openbao-operator-system`","Namespace that hosts the central `OpenBaoTenant` resource."]}]}),"\n",(0,s.jsx)(a.h2,{id:"step-1-create-the-dedicated-public-passthrough-gateway",children:"Step 1: Create the dedicated public passthrough Gateway"}),"\n",(0,s.jsx)(r,{language:"yaml",label:"apply",title:"Expose the hardened hostname through a dedicated TLS passthrough Gateway",code:`apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
name: <gateway-name>
namespace: <gateway-namespace>
spec:
gatewayClassName: <gateway-class-name>
listeners:
  - name: websecure-passthrough
    hostname: <external-host>
    port: 443
    protocol: TLS
    tls:
      mode: Passthrough
    allowedRoutes:
      namespaces:
        from: All`}),"\n",(0,s.jsx)(t,{type:"note",title:"Validated edge shape",children:(0,s.jsxs)(a.p,{children:["The validated EKS design used a dedicated Traefik release for the hardened hostname with a public ",(0,s.jsx)(a.code,{children:"LoadBalancer"}),", only port ",(0,s.jsx)(a.code,{children:"443"})," exposed, ",(0,s.jsx)(a.code,{children:"externalTrafficPolicy: Local"}),", and ",(0,s.jsx)(a.code,{children:"TLSRoute"})," support enabled."]})}),"\n",(0,s.jsx)(a.h2,{id:"step-2-onboard-the-tenant-namespace",children:"Step 2: Onboard the tenant namespace"}),"\n",(0,s.jsx)(r,{language:"yaml",label:"apply",title:"Create the namespace, onboarding request, and admin ServiceAccount",code:`apiVersion: v1
kind: Namespace
metadata:
name: <namespace>
labels:
  openbao.org/tenant: "true"
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoTenant
metadata:
name: <cluster-name>-tenant
namespace: <operator-namespace>
spec:
targetNamespace: <namespace>
---
apiVersion: v1
kind: ServiceAccount
metadata:
name: openbao-admin
namespace: <namespace>`}),"\n",(0,s.jsx)(a.h2,{id:"step-3-apply-the-validated-hardened-eks-cluster",children:"Step 3: Apply the validated hardened EKS cluster"}),"\n",(0,s.jsx)(r,{language:"yaml",label:"apply",title:"Apply the Hardened-profile EKS manifest",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: <cluster-name>
namespace: <namespace>
spec:
profile: Hardened
replicas: 3
version: "<openbao-version>"

configuration:
  logLevel: "info"
  ui: true
  logging:
    format: "json"
  defaultLeaseTTL: "720h"
  maxLeaseTTL: "8760h"
  cacheSize: 134217728
  disableCache: false
  raft:
    performanceMultiplier: 2

imageVerification:
  enabled: true
  failurePolicy: Block
operatorImageVerification:
  enabled: true
  failurePolicy: Block

tls:
  enabled: true
  mode: ACME
  acme:
    directoryURL: "https://acme-v02.api.letsencrypt.org/directory"
    domains:
      - "<external-host>"
    email: "platform@example.com"
    sharedCache:
      mode: ManagedPVC
      size: "1Gi"
      storageClassName: <acme-cache-storage-class>

storage:
  size: "10Gi"
  storageClassName: gp3
deletionPolicy: DeleteAll

serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: "<main-role-arn>"

unseal:
  type: awskms
  awskms:
    region: "<aws-region>"
    kmsKeyID: "<kms-key-arn>"

selfInit:
  enabled: true
  oidc:
    enabled: true
  requests:
    - name: enable-jwt-auth
      operation: update
      path: sys/auth/jwt
      authMethod:
        type: jwt
    - name: create-admin-policy
      operation: update
      path: sys/policies/acl/admin
      policy:
        policy: |
          path "*" {
            capabilities = ["create", "read", "update", "delete", "list", "sudo"]
          }
    - name: create-admin-jwt-role
      operation: update
      path: auth/jwt/role/admin
      data:
        role_type: jwt
        user_claim: sub
        bound_audiences:
          - openbao-internal
        bound_subject: system:serviceaccount:<namespace>:openbao-admin
        token_policies:
          - admin
        policies:
          - admin
        ttl: 1h

gateway:
  enabled: true
  listenerName: websecure-passthrough
  gatewayRef:
    name: <gateway-name>
    namespace: <gateway-namespace>
  hostname: "<external-host>"
  tlsPassthrough: true

backup:
  schedule: "0 */6 * * *"
  target:
    provider: s3
    endpoint: "https://s3.<aws-region>.amazonaws.com"
    bucket: "<backup-bucket>"
    pathPrefix: "clusters/<cluster-name>"
    region: "<aws-region>"
    roleArn: "<backup-role-arn>"
    usePathStyle: false
  retention:
    maxCount: 7
    maxAge: "168h"

upgrade:
  preUpgradeSnapshot: true
  strategy: RollingUpdate

network:
  egressRules:
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - protocol: TCP
          port: 443`}),"\n",(0,s.jsx)(t,{type:"tip",title:"Helper image defaults",children:(0,s.jsx)(a.p,{children:"For released operator builds, prefer the default operator-managed helper images or explicitly pin official signed helper images that match your operator release."})}),"\n",(0,s.jsx)(a.h2,{id:"verify-the-lane",children:"Verify the lane"}),"\n",(0,s.jsx)(r,{language:"bash",label:"verify",title:"Check the cluster conditions",code:`kubectl -n <namespace> get openbaocluster <cluster-name> \\
-o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'`,children:(0,s.jsxs)(a.p,{children:["The steady-state expectation is ",(0,s.jsx)(a.code,{children:"Available=True"}),", ",(0,s.jsx)(a.code,{children:"ACMEIntegrationReady=True"}),", ",(0,s.jsx)(a.code,{children:"ACMECacheReady=True"}),", ",(0,s.jsx)(a.code,{children:"CloudUnsealIdentityReady=True"}),", ",(0,s.jsx)(a.code,{children:"BackupConfigurationReady=True"}),", ",(0,s.jsx)(a.code,{children:"ProductionReady=True"}),", ",(0,s.jsx)(a.code,{children:"OpenBaoInitialized=True"}),", and ",(0,s.jsx)(a.code,{children:"OpenBaoSealed=False"}),"."]})}),"\n",(0,s.jsx)(r,{language:"bash",label:"verify",title:"Verify Gateway programming and the public certificate",code:`kubectl -n <gateway-namespace> get gateway <gateway-name> -o yaml
curl -I https://<external-host>`,children:(0,s.jsxs)(a.p,{children:["The Gateway should report ",(0,s.jsx)(a.code,{children:"Accepted=True"})," and ",(0,s.jsx)(a.code,{children:"Programmed=True"}),". The public endpoint should present a valid certificate and an OpenBao response code such as ",(0,s.jsx)(a.code,{children:"307"}),", ",(0,s.jsx)(a.code,{children:"429"}),", or another application-level reply."]})}),"\n",(0,s.jsx)(r,{language:"bash",label:"verify",title:"Verify JWT admin login and trigger a manual backup",code:`JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS \\
-H 'Content-Type: application/json' \\
-d "{\\"role\\":\\"admin\\",\\"jwt\\":\\"\${JWT}\\"}" \\
"https://<external-host>/v1/auth/jwt/login"

kubectl -n <namespace> annotate openbaocluster <cluster-name> \\
openbao.org/trigger-backup="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite

kubectl -n <namespace> get openbaocluster <cluster-name> \\
-o jsonpath='{.status.backup.lastBackupName}{"\\n"}{.status.backup.lastBackupTime}{"\\n"}{.status.backup.lastFailureReason}{"\\n"}{.status.backup.lastFailureMessage}{"\\n"}'`}),"\n",(0,s.jsx)(i,{title:"Keep moving",items:[{label:"Reference architecture",description:"Review the hardened lane summary, topology, and invariants behind this cloud baseline.",docId:"user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme"},{label:"Troubleshoot the cluster",description:"Use the generic incident routing page if ACME, Gateway, or KMS readiness fails to settle.",docId:"user-guide/openbaocluster/operations/troubleshooting"},{label:"Backup operations",description:"Return to the operator-wide backup guide for retention and restore planning beyond this validated lane.",docId:"user-guide/openbaocluster/operations/backups"}]})]})}function p(e={}){let{wrapper:a}={...(0,o.R)(),...e.components};return a?(0,s.jsx)(a,{...e,children:(0,s.jsx)(c,{...e})}):c(e)}function u(e,a){throw Error("Expected "+(a?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},10506(e,a,t){t.d(a,{R:()=>r,x:()=>l});var n=t(12888);let s={},o=n.createContext(s);function r(e){let a=n.useContext(o);return n.useMemo(function(){return"function"==typeof e?e(a):{...a,...e}},[a,e])}function l(e){let a;return a=e.disableParentContext?"function"==typeof e.components?e.components(s):e.components||s:r(e.components),n.createElement(o.Provider,{value:a},e.children)}}}]);
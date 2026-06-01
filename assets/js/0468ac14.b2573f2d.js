"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["32047"],{48947(e,a,t){t.r(a),t.d(a,{metadata:()=>n,default:()=>c,frontMatter:()=>r,contentTitle:()=>i,toc:()=>d,assets:()=>l});var n=JSON.parse('{"id":"user-guide/validated-deployments/recipes/cloud/amazon-eks-development-awskms-s3","title":"EKS Development / Shared Edge Recipe","description":"Reproduce the validated development baseline on Amazon EKS with AWS KMS auto-unseal, a shared terminating Gateway, JWT bootstrap, and S3 backups.","source":"@site/versioned_docs/version-0.2.0/user-guide/validated-deployments/recipes/cloud/amazon-eks-development-awskms-s3.md","sourceDirName":"user-guide/validated-deployments/recipes/cloud","slug":"/user-guide/validated-deployments/recipes/cloud/amazon-eks-development-awskms-s3","permalink":"/openbao-operator/docs/0.2.0/user-guide/validated-deployments/recipes/cloud/amazon-eks-development-awskms-s3","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/validated-deployments/recipes/cloud/amazon-eks-development-awskms-s3.md","tags":[],"version":"0.2.0","lastUpdatedBy":"openbao-operator-release-pr[bot]","lastUpdatedAt":1777747400000,"frontMatter":{"title":"EKS Development / Shared Edge Recipe","hide_title":true,"pageType":"task","journey":"validated-deployments","description":"Reproduce the validated development baseline on Amazon EKS with AWS KMS auto-unseal, a shared terminating Gateway, JWT bootstrap, and S3 backups."},"sidebar":"operatorDocs","previous":{"title":"Reference architecture","permalink":"/openbao-operator/docs/0.2.0/user-guide/validated-deployments/architectures/cloud/amazon-eks-development-awskms-s3"},"next":{"title":"Reference architecture","permalink":"/openbao-operator/docs/0.2.0/user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme"}}'),o=t(74848),s=t(28453);let r={title:"EKS Development / Shared Edge Recipe",hide_title:!0,pageType:"task",journey:"validated-deployments",description:"Reproduce the validated development baseline on Amazon EKS with AWS KMS auto-unseal, a shared terminating Gateway, JWT bootstrap, and S3 backups."},i,l={},d=[{value:"Step 1: Onboard the tenant namespace",id:"step-1-onboard-the-tenant-namespace",level:2},{value:"Step 2: Apply the validated EKS development cluster",id:"step-2-apply-the-validated-eks-development-cluster",level:2},{value:"Verify the lane",id:"verify-the-lane",level:2}];function p(e){let a={code:"code",h2:"h2",p:"p",...(0,s.R)(),...e.components},{Callout:t,Checklist:n,CommandBlock:r,DecisionTable:i,NextActions:l,PageHeader:d,SiteLink:p}=a;return t||u("Callout",!0),n||u("Checklist",!0),r||u("CommandBlock",!0),i||u("DecisionTable",!0),l||u("NextActions",!0),d||u("PageHeader",!0),p||u("SiteLink",!0),(0,o.jsxs)(o.Fragment,{children:[(0,o.jsx)(d,{title:"Reproduce the validated EKS development lane",lede:"This recipe applies the EKS development baseline with KMS auto-unseal, shared-edge exposure, JWT bootstrap, and S3 backups. Use it when you need the exact validated cloud bring-up path for this topology."}),"\n",(0,o.jsx)(n,{title:"Recipe outcomes",items:["an onboarded tenant namespace and admin ServiceAccount","a Development-profile cluster that unseals with AWS KMS through workload identity","JWT admin login and shared-edge access working on the public hostname you chose","manual and scheduled S3 backups using a backup identity that stays separate from the main workload identity"]}),"\n",(0,o.jsx)(t,{type:"success",title:"Validated coverage",children:(0,o.jsx)(a.p,{children:"This recipe matches the EKS development lane validated in the project cloud environment. The tested path covered KMS unseal, JWT bootstrap, Gateway exposure, and successful S3 backups."})}),"\n",(0,o.jsx)(t,{type:"note",title:"Use the main docs for generic operator behavior",children:(0,o.jsxs)(a.p,{children:["Use ",(0,o.jsx)(p,{docId:"user-guide/openbaotenant/onboarding",children:"tenant onboarding"}),", ",(0,o.jsx)(p,{docId:"user-guide/openbaocluster/configuration/gateway-api",children:"Gateway API support"}),", and ",(0,o.jsx)(p,{docId:"user-guide/openbaocluster/operations/backups",children:"backup operations"})," for the product-wide guidance. This recipe documents the validated lane."]})}),"\n",(0,o.jsx)(i,{title:"Baseline assumptions",columns:["Assumption","Why it exists","What breaks if it is wrong"],rows:[{cells:["EKS has IRSA or an equivalent workload identity path enabled","Both KMS unseal and S3 backup identities depend on cloud workload identity behavior.","The main workload or backup jobs will fail authentication even if the manifests themselves look correct."],emphasis:"recommended"},{cells:["The shared Gateway terminates HTTPS and can re-encrypt to OpenBao","The lane uses a terminating edge instead of passthrough.","If you switch to passthrough or plain HTTP forwarding, you are no longer reproducing the validated baseline."]},{cells:["Separate AWS identities exist for unseal and backup","The validated lane treats those permissions as distinct surfaces.","You will miss the exact failure modes and permission boundaries the lane is supposed to prove."]},{cells:["This remains a Development profile","The lane is intentionally optimized for bring-up and validation speed.","Treating it as a production profile will create the wrong expectations around `ProductionReady` and endpoint posture."],emphasis:"caution"}]}),"\n",(0,o.jsx)(i,{kind:"reference",title:"Inputs to replace before apply",columns:["Placeholder","Example","Purpose"],rows:[{cells:["`<namespace>`","`openbaocluster-dev`","Tenant namespace for the cluster."]},{cells:["`<cluster-name>`","`openbaocluster-dev`","`OpenBaoCluster` name."]},{cells:["`<openbao-version>`","`2.5.1`","OpenBao version."]},{cells:["`<aws-region>`","`eu-central-1`","AWS region for KMS and S3."]},{cells:["`<kms-key-arn>`","`arn:aws:kms:...`","KMS key ARN for auto-unseal."]},{cells:["`<main-role-arn>`","`arn:aws:iam::...:role/openbao-unseal`","IRSA role for the main OpenBao Pods."]},{cells:["`<backup-role-arn>`","`arn:aws:iam::...:role/openbao-backup`","IRSA role for backup Jobs."]},{cells:["`<backup-bucket>`","`openbao-backups`","S3 bucket for snapshots."]},{cells:["`<gateway-name>`","`shared-gateway`","Existing terminating Gateway."]},{cells:["`<gateway-namespace>`","`default`","Namespace of the Gateway."]},{cells:["`<external-host>`","`bao-dev.example.com`","External hostname for the development cluster."]},{cells:["`<operator-namespace>`","`openbao-operator-system`","Namespace that hosts the central `OpenBaoTenant` resource."]}]}),"\n",(0,o.jsx)(a.h2,{id:"step-1-onboard-the-tenant-namespace",children:"Step 1: Onboard the tenant namespace"}),"\n",(0,o.jsx)(r,{language:"yaml",label:"apply",title:"Create the namespace, onboarding request, and admin ServiceAccount",code:`apiVersion: v1
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
namespace: <namespace>`}),"\n",(0,o.jsx)(r,{language:"bash",label:"verify",title:"Wait for tenant provisioning",code:"kubectl -n <operator-namespace> describe openbaotenant <cluster-name>-tenant",children:(0,o.jsxs)(a.p,{children:["The steady-state expectation is ",(0,o.jsx)(a.code,{children:"Provisioned=True"}),"."]})}),"\n",(0,o.jsx)(a.h2,{id:"step-2-apply-the-validated-eks-development-cluster",children:"Step 2: Apply the validated EKS development cluster"}),"\n",(0,o.jsx)(r,{language:"yaml",label:"apply",title:"Apply the Development-profile EKS manifest",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: <cluster-name>
namespace: <namespace>
spec:
profile: Development
replicas: 3
version: "<openbao-version>"

workloadHardening:
  appArmorEnabled: false

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

tls:
  enabled: true
  mode: External

storage:
  size: "10Gi"
  storageClassName: gp3
deletionPolicy: DeleteAll

imageVerification:
  enabled: false
  failurePolicy: Block
operatorImageVerification:
  enabled: false
  failurePolicy: Block

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
  listenerName: websecure
  gatewayRef:
    name: <gateway-name>
    namespace: <gateway-namespace>
  hostname: "<external-host>"
  backendTLS:
    enabled: true
  tlsPassthrough: false
  path: /

backup:
  schedule: "*/30 * * * *"
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
  strategy: RollingUpdate`}),"\n",(0,o.jsx)(t,{type:"note",title:"AppArmor on EKS",children:(0,o.jsxs)(a.p,{children:["The validated EKS lane set ",(0,o.jsx)(a.code,{children:"spec.workloadHardening.appArmorEnabled: false"}),". Remove that override only if your node OS and runtime support AppArmor cleanly."]})}),"\n",(0,o.jsx)(a.h2,{id:"verify-the-lane",children:"Verify the lane"}),"\n",(0,o.jsx)(r,{language:"bash",label:"verify",title:"Check the cluster conditions",code:`kubectl -n <namespace> get openbaocluster <cluster-name> \\
-o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'`,children:(0,o.jsxs)(a.p,{children:["The steady-state expectation is ",(0,o.jsx)(a.code,{children:"Available=True"}),", ",(0,o.jsx)(a.code,{children:"CloudUnsealIdentityReady=True"}),", ",(0,o.jsx)(a.code,{children:"BackupConfigurationReady=True"}),", ",(0,o.jsx)(a.code,{children:"GatewayIntegrationReady=True"})," or ",(0,o.jsx)(a.code,{children:"Unknown"}),", ",(0,o.jsx)(a.code,{children:"OpenBaoInitialized=True"}),", and ",(0,o.jsx)(a.code,{children:"OpenBaoSealed=False"}),"."]})}),"\n",(0,o.jsx)(r,{language:"bash",label:"verify",title:"Check the external endpoint and JWT admin login",code:`curl -kI https://<external-host>/v1/sys/health

JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \\
-H 'Content-Type: application/json' \\
-d "{\\"role\\":\\"admin\\",\\"jwt\\":\\"\${JWT}\\"}" \\
"https://<external-host>/v1/auth/jwt/login"`}),"\n",(0,o.jsx)(r,{language:"bash",label:"verify",title:"Trigger and inspect a manual backup",code:`kubectl -n <namespace> annotate openbaocluster <cluster-name> \\
openbao.org/trigger-backup="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite

kubectl -n <namespace> get openbaocluster <cluster-name> \\
-o jsonpath='{.status.backup.lastBackupName}{"\\n"}{.status.backup.lastBackupTime}{"\\n"}{.status.backup.lastFailureReason}{"\\n"}{.status.backup.lastFailureMessage}{"\\n"}'`}),"\n",(0,o.jsx)(l,{title:"Keep moving",items:[{label:"Reference architecture",description:"Review the lane summary, topology, and invariants behind the EKS development path.",docId:"user-guide/validated-deployments/architectures/cloud/amazon-eks-development-awskms-s3"},{label:"Backup operations",description:"Use the operator-wide backup guide for retention, credentials, and restore planning beyond this validated lane.",docId:"user-guide/openbaocluster/operations/backups"},{label:"EKS Hardened",description:"Move to the hardened cloud baseline when you are ready for ACME, passthrough, and a production-style edge.",docId:"user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme"}]})]})}function c(e={}){let{wrapper:a}={...(0,s.R)(),...e.components};return a?(0,o.jsx)(a,{...e,children:(0,o.jsx)(p,{...e})}):p(e)}function u(e,a){throw Error("Expected "+(a?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},28453(e,a,t){t.d(a,{R:()=>r,x:()=>i});var n=t(96540);let o={},s=n.createContext(o);function r(e){let a=n.useContext(s);return n.useMemo(function(){return"function"==typeof e?e(a):{...a,...e}},[a,e])}function i(e){let a;return a=e.disableParentContext?"function"==typeof e.components?e.components(o):e.components||o:r(e.components),n.createElement(s.Provider,{value:a},e.children)}}}]);
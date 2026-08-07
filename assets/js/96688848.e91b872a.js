"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["75437"],{76234(e,a,t){t.r(a),t.d(a,{metadata:()=>n,default:()=>p,frontMatter:()=>r,contentTitle:()=>l,toc:()=>d,assets:()=>i});var n=JSON.parse('{"id":"user-guide/validated-deployments/recipes/local/development-self-init-userpass","title":"k3d Development / Shared Edge Recipe","description":"Reproduce the validated local development lane with self-init, operator-managed TLS, a shared terminating edge, demo login, JWT admin access, and RustFS backups.","source":"@site/versioned_docs/version-0.1.0/user-guide/validated-deployments/recipes/local/development-self-init-userpass.md","sourceDirName":"user-guide/validated-deployments/recipes/local","slug":"/user-guide/validated-deployments/recipes/local/development-self-init-userpass","permalink":"/openbao-operator/docs/0.1.0/user-guide/validated-deployments/recipes/local/development-self-init-userpass","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/validated-deployments/recipes/local/development-self-init-userpass.md","tags":[],"version":"0.1.0","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1774904343000,"frontMatter":{"title":"k3d Development / Shared Edge Recipe","hide_title":true,"pageType":"task","journey":"validated-deployments","description":"Reproduce the validated local development lane with self-init, operator-managed TLS, a shared terminating edge, demo login, JWT admin access, and RustFS backups."},"sidebar":"operatorDocs","previous":{"title":"Reference architecture","permalink":"/openbao-operator/docs/0.1.0/user-guide/validated-deployments/architectures/local/k3d-development-shared-edge-rustfs"},"next":{"title":"Reference architecture","permalink":"/openbao-operator/docs/0.1.0/user-guide/validated-deployments/architectures/local/k3d-hardened-transit-external-tls"}}'),s=t(91987),o=t(67008);let r={title:"k3d Development / Shared Edge Recipe",hide_title:!0,pageType:"task",journey:"validated-deployments",description:"Reproduce the validated local development lane with self-init, operator-managed TLS, a shared terminating edge, demo login, JWT admin access, and RustFS backups."},l,i={},d=[{value:"Step 1: Onboard the tenant namespace",id:"step-1-onboard-the-tenant-namespace",level:2},{value:"Step 2: Create the backup credentials Secret",id:"step-2-create-the-backup-credentials-secret",level:2},{value:"Step 3: Apply the validated development cluster manifest",id:"step-3-apply-the-validated-development-cluster-manifest",level:2},{value:"Verify the lane",id:"verify-the-lane",level:2}];function c(e){let a={code:"code",h2:"h2",p:"p",pre:"pre",...(0,o.R)(),...e.components},{Callout:t,Checklist:n,CommandBlock:r,DecisionTable:l,NextActions:i,PageHeader:d,SiteLink:c}=a;return t||u("Callout",!0),n||u("Checklist",!0),r||u("CommandBlock",!0),l||u("DecisionTable",!0),i||u("NextActions",!0),d||u("PageHeader",!0),c||u("SiteLink",!0),(0,s.jsxs)(s.Fragment,{children:[(0,s.jsx)(d,{title:"Reproduce the validated local development lane without turning a quick-start cluster into a pile of one-off overrides.",lede:"This recipe stands up the local development baseline with tenant onboarding, operator-managed TLS, a shared terminating edge, JWT bootstrap, an optional demo login, and an S3-compatible backup path backed by RustFS."}),"\n",(0,s.jsx)(n,{title:"This recipe should leave you with",items:["an onboarded tenant namespace and admin ServiceAccount","a Development-profile cluster that self-initializes and exposes the UI through the shared edge","JWT admin login working for a real ServiceAccount token","a RustFS-backed backup configuration you can verify before the first upgrade rehearsal"]}),"\n",(0,s.jsx)(t,{type:"success",title:"Validated lane",children:(0,s.jsxs)(a.p,{children:["This recipe follows the development lifecycle, backup, and blue/green patterns exercised in the local validation environment and the in-repo E2E suites. The optional ",(0,s.jsx)(a.code,{children:"userpass"})," login remains local-only convenience layered on top of the validated cluster path."]})}),"\n",(0,s.jsx)(t,{type:"note",title:"Use the main docs for the product-wide source of truth",children:(0,s.jsxs)(a.p,{children:["Use ",(0,s.jsx)(c,{docId:"user-guide/openbaotenant/onboarding",children:"tenant onboarding"}),", ",(0,s.jsx)(c,{docId:"user-guide/openbaocluster/configuration/external-access",children:"external access"}),", and ",(0,s.jsx)(c,{docId:"user-guide/openbaocluster/operations/backups",children:"backup operations"})," when you need the generic operator behavior. This recipe only captures the exact validated local lane."]})}),"\n",(0,s.jsx)(l,{title:"What this lane assumes",columns:["Assumption","Why it exists","What breaks if it is wrong"],rows:[{cells:["Multi-tenant operator install with admission enabled","The validated path starts from the default tenant-onboarding model.","Namespace provisioning and generated RBAC will drift from the lane you are trying to reproduce."],emphasis:"recommended"},{cells:["A shared terminating Gateway already exists","The lane intentionally reuses one local edge for OpenBao and the rest of the toolchain.","You will not validate the same routing contract if you fall back to port-forwarding or user-managed passthrough."]},{cells:["RustFS is reachable as an S3-compatible endpoint","Backups are part of the lane, not an optional afterthought.","The cluster may look healthy while the part of the lane that matters for restore rehearsal never actually works."]},{cells:["Demo login stays local-only","The `userpass` bootstrap is included only to make UI validation and demos faster.","Reusing it outside a disposable environment turns a convenience into a security mistake."],emphasis:"caution"}]}),"\n",(0,s.jsx)(l,{kind:"reference",title:"Inputs to replace before apply",columns:["Placeholder","Example","Purpose"],rows:[{cells:["`<namespace>`","`openbaocluster-demo`","Tenant namespace for the cluster."]},{cells:["`<cluster-name>`","`openbaocluster-demo`","`OpenBaoCluster` name."]},{cells:["`<openbao-version>`","`2.5.1`","OpenBao version."]},{cells:["`<gateway-name>`","`shared-gateway`","Existing terminating Gateway used by the local toolchain."]},{cells:["`<gateway-namespace>`","`default`","Namespace of the Gateway."]},{cells:["`<external-host>`","`bao-demo.example.com`","External hostname for the shared-edge route."]},{cells:["`<operator-namespace>`","`openbao-operator-system`","Namespace that hosts the central `OpenBaoTenant` resource."]}]}),"\n",(0,s.jsx)(a.h2,{id:"step-1-onboard-the-tenant-namespace",children:"Step 1: Onboard the tenant namespace"}),"\n",(0,s.jsx)(r,{language:"yaml",label:"apply",title:"Create the namespace, onboarding request, and admin ServiceAccount",code:`apiVersion: v1
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
namespace: <namespace>`}),"\n",(0,s.jsx)(r,{language:"bash",label:"verify",title:"Wait for tenant provisioning",code:"kubectl -n <operator-namespace> describe openbaotenant <cluster-name>-tenant",children:(0,s.jsxs)(a.p,{children:["The steady-state expectation is ",(0,s.jsx)(a.code,{children:"Provisioned=True"}),". Do not move to the cluster manifest until the namespace is actually prepared for the operator."]})}),"\n",(0,s.jsx)(a.h2,{id:"step-2-create-the-backup-credentials-secret",children:"Step 2: Create the backup credentials Secret"}),"\n",(0,s.jsx)(r,{language:"bash",label:"apply",title:"Create the RustFS credentials Secret",code:`kubectl -n <namespace> create secret generic rustfs-secret \\
--from-literal=accessKeyId='rustfsadmin' \\
--from-literal=secretAccessKey='rustfsadmin'`,children:(0,s.jsx)(a.p,{children:"If your local RustFS instance uses different credentials, replace both values here and in the object-storage service itself."})}),"\n",(0,s.jsx)(a.h2,{id:"step-3-apply-the-validated-development-cluster-manifest",children:"Step 3: Apply the validated development cluster manifest"}),"\n",(0,s.jsx)(r,{language:"yaml",label:"apply",title:"Apply the Development-profile cluster",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: <cluster-name>
namespace: <namespace>
spec:
profile: Development
replicas: 3
version: "<openbao-version>"

tls:
  enabled: true
  mode: OperatorManaged
  rotationPeriod: "720h"

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

storage:
  size: "10Gi"
deletionPolicy: DeleteAll

selfInit:
  enabled: true
  oidc:
    enabled: true
  requests:
    - name: enable-userpass-auth
      operation: update
      path: sys/auth/userpass
      authMethod:
        type: userpass
    - name: enable-jwt-auth
      operation: update
      path: sys/auth/jwt
      authMethod:
        type: jwt
    - name: enable-demo-kv
      operation: update
      path: sys/mounts/secret
      secretEngine:
        type: kv
        description: "Demo KV v2 engine"
        options:
          version: "2"
    - name: create-admin-policy
      operation: update
      path: sys/policies/acl/admin
      policy:
        policy: |
          path "*" {
            capabilities = ["create", "read", "update", "delete", "list", "sudo"]
          }
    - name: create-demo-ui-user
      operation: update
      path: auth/userpass/users/demo-admin
      data:
        password: "demo-password"
        token_policies:
          - admin
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
    endpoint: "http://rustfs-svc.rustfs.svc.cluster.local:9000"
    bucket: "openbao-backups"
    pathPrefix: "clusters/<cluster-name>"
    usePathStyle: true
    credentialsSecretRef:
      name: rustfs-secret
  retention:
    maxCount: 7
    maxAge: "168h"

upgrade:
  preUpgradeSnapshot: true
  strategy: BlueGreen`}),"\n",(0,s.jsx)(t,{type:"warning",title:"Demo-only credentials",children:(0,s.jsxs)(a.p,{children:["The ",(0,s.jsx)(a.code,{children:"demo-admin"})," user exists only to make local validation easy. Keep it out of any shared or long-lived environment."]})}),"\n",(0,s.jsxs)(t,{type:"note",title:"AppArmor on local clusters",children:[(0,s.jsx)(a.p,{children:"If kubelet rejects the Pods because AppArmor is unavailable, add:"}),(0,s.jsx)(a.pre,{children:(0,s.jsx)(a.code,{className:"language-yaml",children:"spec:\n  workloadHardening:\n    appArmorEnabled: false\n"})})]}),"\n",(0,s.jsx)(a.h2,{id:"verify-the-lane",children:"Verify the lane"}),"\n",(0,s.jsx)(r,{language:"bash",label:"verify",title:"Check the cluster conditions",code:`kubectl -n <namespace> get openbaocluster <cluster-name> \\
-o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'`,children:(0,s.jsxs)(a.p,{children:["The steady-state expectation is ",(0,s.jsx)(a.code,{children:"Available=True"}),", ",(0,s.jsx)(a.code,{children:"TLSReady=True"}),", ",(0,s.jsx)(a.code,{children:"UserAccessBootstrap=True"}),", ",(0,s.jsx)(a.code,{children:"BackupConfigurationReady=True"}),", ",(0,s.jsx)(a.code,{children:"GatewayIntegrationReady=True"}),", ",(0,s.jsx)(a.code,{children:"OpenBaoInitialized=True"}),", and ",(0,s.jsx)(a.code,{children:"OpenBaoSealed=False"}),"."]})}),"\n",(0,s.jsx)(r,{language:"bash",label:"verify",title:"Confirm the cluster did not persist a root token Secret",code:"kubectl -n <namespace> get secret <cluster-name>-root-token",children:(0,s.jsxs)(a.p,{children:["This should return ",(0,s.jsx)(a.code,{children:"NotFound"}),". A self-init lane should not leave the root token stored as a Kubernetes Secret."]})}),"\n",(0,s.jsx)(r,{language:"bash",label:"verify",title:"Verify the demo UI login through the local service",code:`kubectl -n <namespace> port-forward svc/<cluster-name> 8200:8200
export VAULT_ADDR="https://127.0.0.1:8200"

curl -sS -k \\
-H 'Content-Type: application/json' \\
-d '{"password":"demo-password"}' \\
\${VAULT_ADDR%/}/v1/auth/userpass/login/demo-admin`,children:(0,s.jsx)(a.p,{children:"Local browsers and CLIs may warn about the operator-managed CA. That is expected in this lane."})}),"\n",(0,s.jsx)(r,{language:"bash",label:"verify",title:"Verify JWT admin login",code:`JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \\
-H 'Content-Type: application/json' \\
-d "{\\"role\\":\\"admin\\",\\"jwt\\":\\"\${JWT}\\"}" \\
\${VAULT_ADDR%/}/v1/auth/jwt/login`}),"\n",(0,s.jsx)(r,{language:"bash",label:"verify",title:"Trigger and inspect a manual backup",code:`kubectl -n <namespace> annotate openbaocluster <cluster-name> \\
openbao.org/trigger-backup="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite

kubectl -n <namespace> get openbaocluster <cluster-name> \\
-o jsonpath='{.status.backup.lastBackupName}{"\\n"}{.status.backup.lastBackupTime}{"\\n"}{.status.backup.lastFailureReason}{"\\n"}{.status.backup.lastFailureMessage}{"\\n"}'`,children:(0,s.jsx)(a.p,{children:"A successful lane should produce a backup object key and no failure reason or failure message."})}),"\n",(0,s.jsx)(i,{title:"Keep moving",items:[{label:"Reference architecture",description:"Review the lane summary, topology, and invariants behind the recipe you just applied.",docId:"user-guide/validated-deployments/architectures/local/k3d-development-shared-edge-rustfs"},{label:"Backup operations",description:"Expand the RustFS-specific recipe choices into the generic backup model used by the operator.",docId:"user-guide/openbaocluster/operations/backups"},{label:"k3d Hardened / External TLS",description:"Move to the hardened local rehearsal lane when you need external certificates and an external unseal root.",docId:"user-guide/validated-deployments/architectures/local/k3d-hardened-transit-external-tls"}]})]})}function p(e={}){let{wrapper:a}={...(0,o.R)(),...e.components};return a?(0,s.jsx)(a,{...e,children:(0,s.jsx)(c,{...e})}):c(e)}function u(e,a){throw Error("Expected "+(a?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},67008(e,a,t){t.d(a,{R:()=>r,x:()=>l});var n=t(71763);let s={},o=n.createContext(s);function r(e){let a=n.useContext(o);return n.useMemo(function(){return"function"==typeof e?e(a):{...a,...e}},[a,e])}function l(e){let a;return a=e.disableParentContext?"function"==typeof e.components?e.components(s):e.components||s:r(e.components),n.createElement(o.Provider,{value:a},e.children)}}}]);
"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["91384"],{90127(e,a,t){t.r(a),t.d(a,{metadata:()=>n,default:()=>p,frontMatter:()=>o,contentTitle:()=>i,toc:()=>c,assets:()=>l});var n=JSON.parse('{"id":"user-guide/validated-deployments/recipes/local/hardened-transit-external-tls","title":"k3d Hardened / External TLS Recipe","description":"Reproduce the validated local hardened lane with Transit auto-unseal, externally managed TLS Secrets, self-init, and user-managed passthrough access.","source":"@site/versioned_docs/version-0.1.0/user-guide/validated-deployments/recipes/local/hardened-transit-external-tls.md","sourceDirName":"user-guide/validated-deployments/recipes/local","slug":"/user-guide/validated-deployments/recipes/local/hardened-transit-external-tls","permalink":"/openbao-operator/docs/0.1.0/user-guide/validated-deployments/recipes/local/hardened-transit-external-tls","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/validated-deployments/recipes/local/hardened-transit-external-tls.md","tags":[],"version":"0.1.0","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1774904343000,"frontMatter":{"title":"k3d Hardened / External TLS Recipe","hide_title":true,"pageType":"task","journey":"validated-deployments","description":"Reproduce the validated local hardened lane with Transit auto-unseal, externally managed TLS Secrets, self-init, and user-managed passthrough access."},"sidebar":"operatorDocs","previous":{"title":"Reference architecture","permalink":"/openbao-operator/docs/0.1.0/user-guide/validated-deployments/architectures/local/k3d-hardened-transit-external-tls"},"next":{"title":"Reference architecture","permalink":"/openbao-operator/docs/0.1.0/user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme"}}'),s=t(11684),r=t(74383);let o={title:"k3d Hardened / External TLS Recipe",hide_title:!0,pageType:"task",journey:"validated-deployments",description:"Reproduce the validated local hardened lane with Transit auto-unseal, externally managed TLS Secrets, self-init, and user-managed passthrough access."},i,l={},c=[{value:"Step 1: Onboard the tenant namespace",id:"step-1-onboard-the-tenant-namespace",level:2},{value:"Step 2: Create the Transit credential Secret",id:"step-2-create-the-transit-credential-secret",level:2},{value:"Step 3: Create the external TLS Secrets",id:"step-3-create-the-external-tls-secrets",level:2},{value:"Step 4: Apply the OpenBaoCluster",id:"step-4-apply-the-openbaocluster",level:2},{value:"Step 5: Expose the passthrough path",id:"step-5-expose-the-passthrough-path",level:2},{value:"Verify the lane",id:"verify-the-lane",level:2}];function d(e){let a={code:"code",h2:"h2",p:"p",pre:"pre",...(0,r.R)(),...e.components},{Callout:t,Checklist:n,CommandBlock:o,DecisionTable:i,NextActions:l,PageHeader:c,SiteLink:d}=a;return t||u("Callout",!0),n||u("Checklist",!0),o||u("CommandBlock",!0),i||u("DecisionTable",!0),l||u("NextActions",!0),c||u("PageHeader",!0),d||u("SiteLink",!0),(0,s.jsxs)(s.Fragment,{children:[(0,s.jsx)(c,{title:"Reproduce the validated hardened local lane without collapsing the trust boundaries it depends on.",lede:"This recipe stands up the local hardened lane with tenant onboarding, external Transit unseal, externally managed TLS Secrets, and user-managed TCP passthrough. Use it when you want the exact validated path, not a generic local example."}),"\n",(0,s.jsx)(n,{title:"This recipe should leave you with",items:["an onboarded tenant namespace and admin ServiceAccount","a hardened cluster that self-initializes and never persists a root token Secret","Transit auto-unseal working against the external provider you chose for the lane","externally managed TLS Secrets and end-to-end passthrough traffic working together"]}),"\n",(0,s.jsx)(t,{type:"success",title:"Validated lane",children:(0,s.jsx)(a.p,{children:"This recipe follows the hardened external-TLS lifecycle covered by the in-repo E2E suite and the local validation environment. The tested path includes tenant onboarding, external TLS Secrets, Transit auto-unseal, self-init, and successful JWT admin login."})}),"\n",(0,s.jsx)(t,{type:"note",title:"Canonical operator behavior still lives in the main docs",children:(0,s.jsxs)(a.p,{children:["Use the main guides for the product-wide source of truth on ",(0,s.jsx)(d,{docId:"user-guide/openbaocluster/configuration/security-profiles",children:"security profiles"}),", ",(0,s.jsx)(d,{docId:"user-guide/openbaotenant/onboarding",children:"tenant onboarding"}),", and ",(0,s.jsx)(d,{docId:"user-guide/openbaocluster/configuration/external-access",children:"external access"}),". This recipe only captures the exact validated lane."]})}),"\n",(0,s.jsx)(i,{title:"What this lane assumes",columns:["Assumption","Why it exists","What breaks if it is wrong"],rows:[{cells:["Multi-tenant operator install with admission enabled","The validated lane starts from the standard tenant-onboarding path.","Namespace onboarding and guarded hardened behavior will not match the tested path."],emphasis:"recommended"},{cells:["cert-manager is available","The lane expects external TLS Secrets to be provisioned before the workload depends on them.","The cluster will not reach `TLSReady=True` if the Secrets never appear."]},{cells:["Transit is reachable from the tenant namespace","The lane keeps the seal root external on purpose.","The cluster may initialize but fail to unseal or rejoin correctly after restart."]},{cells:["You know the ingress namespace","The network policy must trust the namespace that forwards passthrough traffic.","Traffic may never reach the public Service even though the cluster itself is healthy."]}]}),"\n",(0,s.jsx)(i,{kind:"reference",title:"Inputs to replace before apply",columns:["Placeholder","Example","Purpose"],rows:[{cells:["`<namespace>`","`openbaocluster-hardened`","Tenant namespace for the cluster."]},{cells:["`<cluster-name>`","`openbaocluster-hardened`","`OpenBaoCluster` name."]},{cells:["`<openbao-version>`","`2.5.0`","OpenBao version."]},{cells:["`<transit-address>`","`https://transit-provider.openbao-infra.svc:8200`","Transit provider URL."]},{cells:["`<transit-key>`","`openbao-unseal`","Transit key name."]},{cells:["`<external-host>`","`bao-hardened.example.com`","External DNS name for clients."]},{cells:["`<ingress-namespace>`","`default`","Namespace of the ingress controller that forwards traffic to OpenBao."]},{cells:["`<transit-namespace>`","`openbao-infra`","Namespace hosting the Transit provider."]},{cells:["`<operator-namespace>`","`openbao-operator-system`","Namespace hosting the central `OpenBaoTenant` resource."]}]}),"\n",(0,s.jsx)(a.h2,{id:"step-1-onboard-the-tenant-namespace",children:"Step 1: Onboard the tenant namespace"}),"\n",(0,s.jsx)(o,{language:"yaml",label:"apply",title:"Create the namespace, onboarding request, and admin ServiceAccount",code:`apiVersion: v1
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
namespace: <namespace>`}),"\n",(0,s.jsx)(o,{language:"bash",label:"verify",title:"Verify tenant provisioning",code:"kubectl -n <operator-namespace> describe openbaotenant <cluster-name>-tenant",children:(0,s.jsxs)(a.p,{children:["The steady-state expectation is ",(0,s.jsx)(a.code,{children:"Provisioned=True"}),". Do not continue until the tenant onboarding path is actually complete."]})}),"\n",(0,s.jsx)(a.h2,{id:"step-2-create-the-transit-credential-secret",children:"Step 2: Create the Transit credential Secret"}),"\n",(0,s.jsx)(o,{language:"bash",label:"apply",title:"Create the Secret used by transit auto-unseal",code:`kubectl -n <namespace> create secret generic transit-provider-token \\
--from-literal=token='<transit-token>' \\
--from-file=ca.crt=/path/to/transit-provider-ca.crt`,children:(0,s.jsxs)(a.p,{children:["For the validated path, the Secret contains ",(0,s.jsx)(a.code,{children:"token"})," for ",(0,s.jsx)(a.code,{children:"VAULT_TOKEN"})," and ",(0,s.jsx)(a.code,{children:"ca.crt"})," for ",(0,s.jsx)(a.code,{children:"VAULT_CACERT"}),"."]})}),"\n",(0,s.jsx)(a.h2,{id:"step-3-create-the-external-tls-secrets",children:"Step 3: Create the external TLS Secrets"}),"\n",(0,s.jsx)(o,{language:"yaml",label:"apply",title:"Issue the TLS CA and server Secrets with cert-manager",code:`apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
name: <cluster-name>-selfsigned-issuer
namespace: <namespace>
spec:
selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
name: <cluster-name>-tls-ca
namespace: <namespace>
spec:
secretName: <cluster-name>-tls-ca
commonName: <cluster-name>-ca
isCA: true
issuerRef:
  kind: Issuer
  name: <cluster-name>-selfsigned-issuer
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
name: <cluster-name>-ca-issuer
namespace: <namespace>
spec:
ca:
  secretName: <cluster-name>-tls-ca
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
name: <cluster-name>-tls-server
namespace: <namespace>
spec:
secretName: <cluster-name>-tls-server
dnsNames:
  - <external-host>
  - openbao-cluster-<cluster-name>.local
  - <cluster-name>.<namespace>.svc
  - "*.<cluster-name>.<namespace>.svc"
  - <cluster-name>-public.<namespace>.svc
issuerRef:
  kind: Issuer
  name: <cluster-name>-ca-issuer`}),"\n",(0,s.jsx)(o,{language:"bash",label:"verify",title:"Wait for the TLS Secrets to become ready",code:`kubectl -n <namespace> wait certificate/<cluster-name>-tls-ca --for=condition=Ready --timeout=5m
kubectl -n <namespace> wait certificate/<cluster-name>-tls-server --for=condition=Ready --timeout=5m`,children:(0,s.jsxs)(a.p,{children:["If you already use a corporate issuer, replace the issuer objects but keep the Secret names ",(0,s.jsx)(a.code,{children:"<cluster-name>-tls-ca"})," and ",(0,s.jsx)(a.code,{children:"<cluster-name>-tls-server"}),"."]})}),"\n",(0,s.jsx)(a.h2,{id:"step-4-apply-the-openbaocluster",children:"Step 4: Apply the OpenBaoCluster"}),"\n",(0,s.jsx)(o,{language:"yaml",label:"apply",title:"Apply the validated hardened cluster manifest",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: <cluster-name>
namespace: <namespace>
spec:
profile: Hardened
replicas: 3
version: "<openbao-version>"

storage:
  size: "10Gi"
deletionPolicy: Retain

tls:
  enabled: true
  mode: External

configuration:
  logLevel: "info"
  ui: true
  logging:
    format: "json"

unseal:
  type: transit
  credentialsSecretRef:
    name: transit-provider-token
  transit:
    address: "<transit-address>"
    mountPath: "transit"
    keyName: "<transit-key>"
    tlsCACert: "/etc/bao/seal-creds/ca.crt"

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

upgrade:
  strategy: RollingUpdate

network:
  trustedIngressPeers:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: <ingress-namespace>
  egressRules:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: <transit-namespace>
      ports:
        - protocol: TCP
          port: 8200`}),"\n",(0,s.jsx)(t,{type:"warning",title:"API server endpoint IPs",children:(0,s.jsxs)(a.p,{children:["If your CNI enforces egress on post-DNAT traffic, you may also need ",(0,s.jsx)(a.code,{children:"spec.network.apiServerEndpointIPs"}),". See ",(0,s.jsx)(d,{docId:"user-guide/openbaocluster/configuration/network",children:"Network configuration"}),"."]})}),"\n",(0,s.jsxs)(t,{type:"note",title:"AppArmor on local clusters",children:[(0,s.jsx)(a.p,{children:"If kubelet rejects the Pods because AppArmor is unavailable, add:"}),(0,s.jsx)(a.pre,{children:(0,s.jsx)(a.code,{className:"language-yaml",children:"spec:\n  workloadHardening:\n    appArmorEnabled: false\n"})})]}),"\n",(0,s.jsx)(a.h2,{id:"step-5-expose-the-passthrough-path",children:"Step 5: Expose the passthrough path"}),"\n",(0,s.jsx)(o,{language:"yaml",label:"apply",title:"Create the validated Traefik passthrough route",code:`apiVersion: traefik.io/v1alpha1
kind: IngressRouteTCP
metadata:
name: bao-hardened
namespace: <namespace>
spec:
entryPoints:
  - websecure
routes:
  - match: HostSNI(\`<external-host>\`)
    services:
      - name: <cluster-name>-public
        port: 8200
tls:
  passthrough: true`,children:(0,s.jsxs)(a.p,{children:["This lane uses a user-managed Traefik ",(0,s.jsx)(a.code,{children:"IngressRouteTCP"}),", not ",(0,s.jsx)(a.code,{children:"spec.gateway"}),". Keep the passthrough route separate from any shared terminating edge."]})}),"\n",(0,s.jsx)(a.h2,{id:"verify-the-lane",children:"Verify the lane"}),"\n",(0,s.jsx)(o,{language:"bash",label:"verify",title:"Check the cluster conditions",code:`kubectl -n <namespace> get openbaocluster <cluster-name> \\
-o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'`,children:(0,s.jsxs)(a.p,{children:["The steady-state expectation is ",(0,s.jsx)(a.code,{children:"Available=True"}),", ",(0,s.jsx)(a.code,{children:"TLSReady=True"}),", ",(0,s.jsx)(a.code,{children:"UserAccessBootstrap=True"}),", ",(0,s.jsx)(a.code,{children:"ProductionReady=True"}),", and ",(0,s.jsx)(a.code,{children:"OpenBaoInitialized=True"}),"."]})}),"\n",(0,s.jsx)(o,{language:"bash",label:"verify",title:"Confirm the cluster did not persist a root token Secret",code:"kubectl -n <namespace> get secret <cluster-name>-root-token",children:(0,s.jsxs)(a.p,{children:["This should return ",(0,s.jsx)(a.code,{children:"NotFound"}),". A hardened self-init lane should not leave the root token stored as a Kubernetes Secret."]})}),"\n",(0,s.jsx)(o,{language:"bash",label:"verify",title:"Exchange a Kubernetes JWT for an OpenBao admin token",code:`kubectl -n <namespace> port-forward svc/<cluster-name> 8200:8200
export VAULT_ADDR="https://127.0.0.1:8200"
JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \\
-H 'Content-Type: application/json' \\
-d "{"role":"admin","jwt":"\${JWT}"}" \\
"\${VAULT_ADDR%/}/v1/auth/jwt/login"`,children:(0,s.jsxs)(a.p,{children:["The validated local path uses self-signed certificates, so the example uses ",(0,s.jsx)(a.code,{children:"-k"}),". In a real environment with trusted certificates, remove that shortcut."]})}),"\n",(0,s.jsx)(t,{type:"note",title:"What matters most in this lane",children:(0,s.jsxs)(a.p,{children:["The important exposure contract here is successful end-to-end passthrough plus the ",(0,s.jsx)(a.code,{children:"trustedIngressPeers"})," rule. ",(0,s.jsx)(a.code,{children:"GatewayIntegrationReady"})," is not the primary signal because the route is intentionally managed outside ",(0,s.jsx)(a.code,{children:"spec.gateway"}),"."]})}),"\n",(0,s.jsx)(l,{title:"After the lane is running",items:[{label:"Plan upgrades",description:"Move into the generic operating path once the validated lane is healthy.",docId:"user-guide/openbaocluster/operations/upgrades"},{label:"Configure backups",description:"Add snapshot coverage before you use the lane as a hardened rehearsal environment.",docId:"user-guide/openbaocluster/operations/backups"},{label:"k3d Hardened / ACME",description:"Compare this lane with the local hardened ACME lane when you want OpenBao-managed certificate issuance instead of external TLS Secrets.",docId:"user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme"}]})]})}function p(e={}){let{wrapper:a}={...(0,r.R)(),...e.components};return a?(0,s.jsx)(a,{...e,children:(0,s.jsx)(d,{...e})}):d(e)}function u(e,a){throw Error("Expected "+(a?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},74383(e,a,t){t.d(a,{R:()=>o,x:()=>i});var n=t(12888);let s={},r=n.createContext(s);function o(e){let a=n.useContext(r);return n.useMemo(function(){return"function"==typeof e?e(a):{...a,...e}},[a,e])}function i(e){let a;return a=e.disableParentContext?"function"==typeof e.components?e.components(s):e.components||s:o(e.components),n.createElement(r.Provider,{value:a},e.children)}}}]);
"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["37919"],{15054(e,t,a){a.r(t),a.d(t,{metadata:()=>n,default:()=>p,frontMatter:()=>i,contentTitle:()=>o,toc:()=>c,assets:()=>l});var n=JSON.parse('{"id":"user-guide/validated-deployments/recipes/local/hardened-transit-acme-tls","title":"k3d Hardened / ACME Recipe","description":"Reproduce the validated local hardened ACME lane with Transit auto-unseal, a private ACME issuer, self-init, and user-managed TLS passthrough.","source":"@site/versioned_docs/version-0.4.0/user-guide/validated-deployments/recipes/local/hardened-transit-acme-tls.md","sourceDirName":"user-guide/validated-deployments/recipes/local","slug":"/user-guide/validated-deployments/recipes/local/hardened-transit-acme-tls","permalink":"/openbao-operator/docs/user-guide/validated-deployments/recipes/local/hardened-transit-acme-tls","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/validated-deployments/recipes/local/hardened-transit-acme-tls.md","tags":[],"version":"0.4.0","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1783172994000,"frontMatter":{"title":"k3d Hardened / ACME Recipe","hide_title":true,"pageType":"task","journey":"validated-deployments","description":"Reproduce the validated local hardened ACME lane with Transit auto-unseal, a private ACME issuer, self-init, and user-managed TLS passthrough."},"sidebar":"operatorDocs","previous":{"title":"Reference architecture","permalink":"/openbao-operator/docs/user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme"},"next":{"title":"Reference architecture","permalink":"/openbao-operator/docs/user-guide/validated-deployments/architectures/local/k3d-cross-cluster-dr-transit-rustfs"}}'),s=a(11684),r=a(10506);let i={title:"k3d Hardened / ACME Recipe",hide_title:!0,pageType:"task",journey:"validated-deployments",description:"Reproduce the validated local hardened ACME lane with Transit auto-unseal, a private ACME issuer, self-init, and user-managed TLS passthrough."},o,l={},c=[{value:"Step 1: Onboard the tenant namespace",id:"step-1-onboard-the-tenant-namespace",level:2},{value:"Step 2: Create the Transit and ACME trust Secret",id:"step-2-create-the-transit-and-acme-trust-secret",level:2},{value:"Step 3: Expose the ACME challenge Service with passthrough",id:"step-3-expose-the-acme-challenge-service-with-passthrough",level:2},{value:"Step 4: Apply the hardened ACME cluster",id:"step-4-apply-the-hardened-acme-cluster",level:2},{value:"Verify the lane",id:"verify-the-lane",level:2}];function d(e){let t={code:"code",h2:"h2",p:"p",pre:"pre",...(0,r.R)(),...e.components},{Callout:a,Checklist:n,CommandBlock:i,DecisionTable:o,NextActions:l,PageHeader:c,SiteLink:d}=t;return a||h("Callout",!0),n||h("Checklist",!0),i||h("CommandBlock",!0),o||h("DecisionTable",!0),l||h("NextActions",!0),c||h("PageHeader",!0),d||h("SiteLink",!0),(0,s.jsxs)(s.Fragment,{children:[(0,s.jsx)(c,{title:"Reproduce the validated hardened ACME baseline",lede:"This recipe stands up the local hardened ACME baseline with tenant onboarding, a shared trust-services dependency for Transit and ACME, an internal ACME CA, and a user-managed passthrough route that preserves `tls-alpn-01` behavior."}),"\n",(0,s.jsx)(n,{title:"Recipe outcomes",items:["an onboarded tenant namespace and admin ServiceAccount","a trust Secret that carries both the Transit CA bundle and the private ACME issuer trust root","a hardened cluster that self-initializes, unseals with Transit, and serves ACME traffic through passthrough","conditions and services that confirm ACME is working before you move to a public cloud lane"]}),"\n",(0,s.jsx)(a,{type:"success",title:"Validated coverage",children:(0,s.jsx)(t.p,{children:"This recipe follows the local ACME lifecycle covered by the in-repo ACME suite and the project validation environment. The tested path covers private ACME trust material, Transit auto-unseal, ACME readiness, and human admin JWT access."})}),"\n",(0,s.jsx)(a,{type:"note",title:"Use the main docs for generic behavior",children:(0,s.jsxs)(t.p,{children:["Use ",(0,s.jsx)(d,{docId:"user-guide/openbaocluster/configuration/external-access",children:"external access"}),", ",(0,s.jsx)(d,{docId:"user-guide/openbaocluster/configuration/network",children:"network configuration"}),", and ",(0,s.jsx)(d,{docId:"security/workload/tls",children:"TLS and workload identity"})," for the product-wide explanation. This recipe captures the exact validated local lane."]})}),"\n",(0,s.jsx)(o,{title:"Baseline assumptions",columns:["Assumption","Why it exists","What breaks if it is wrong"],rows:[{cells:["Multi-tenant operator install with admission enabled","The validated path starts from the standard tenant-onboarding flow.","Namespace provisioning and hardened policy enforcement will drift from the tested lane."],emphasis:"recommended"},{cells:["The external trust-services endpoint exposes both Transit and an ACME directory","One dependency owns both the seal root and the private certificate issuance path.","You will not reproduce the lane if Transit and ACME trust material come from unrelated systems."]},{cells:["The external hostname resolves back to the passthrough edge","`tls-alpn-01` only works when the validator reaches the hostname that OpenBao serves.","ACME will fail even though the cluster itself is healthy and the route object exists."]},{cells:["The ingress layer supports TLS passthrough on port `443`","OpenBao must terminate TLS itself in this lane.","Any edge termination in front of OpenBao breaks the ACME contract immediately."],emphasis:"caution"}]}),"\n",(0,s.jsx)(o,{kind:"reference",title:"Inputs to replace before apply",columns:["Placeholder","Example","Purpose"],rows:[{cells:["`<namespace>`","`openbaocluster-acme`","Tenant namespace for the cluster."]},{cells:["`<cluster-name>`","`openbaocluster-acme`","`OpenBaoCluster` name."]},{cells:["`<openbao-version>`","`2.5.0`","OpenBao version."]},{cells:["`<transit-address>`","`https://trust-services.openbao-infra.svc:8200`","Transit provider URL."]},{cells:["`<acme-directory-url>`","`https://trust-services.openbao-infra.svc:8200/v1/pki/acme/directory`","ACME directory URL."]},{cells:["`<transit-key>`","`openbao-unseal`","Transit key name."]},{cells:["`<external-host>`","`bao-acme.example.com`","External hostname used for clients and ACME validation."]},{cells:["`<operator-namespace>`","`openbao-operator-system`","Namespace that hosts the central `OpenBaoTenant` resource."]}]}),"\n",(0,s.jsx)(t.h2,{id:"step-1-onboard-the-tenant-namespace",children:"Step 1: Onboard the tenant namespace"}),"\n",(0,s.jsx)(i,{language:"yaml",label:"apply",title:"Create the namespace, onboarding request, and admin ServiceAccount",code:`apiVersion: v1
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
namespace: <namespace>`}),"\n",(0,s.jsx)(i,{language:"bash",label:"verify",title:"Wait for tenant provisioning",code:"kubectl -n <operator-namespace> describe openbaotenant <cluster-name>-tenant",children:(0,s.jsxs)(t.p,{children:["The steady-state expectation is ",(0,s.jsx)(t.code,{children:"Provisioned=True"}),"."]})}),"\n",(0,s.jsx)(t.h2,{id:"step-2-create-the-transit-and-acme-trust-secret",children:"Step 2: Create the Transit and ACME trust Secret"}),"\n",(0,s.jsx)(i,{language:"bash",label:"apply",title:"Create the Secret used by Transit and the private ACME issuer",code:`kubectl -n <namespace> create secret generic trust-services-token \\
--from-literal=token='<transit-token>' \\
--from-file=ca.crt=/path/to/trust-services-ca.crt \\
--from-file=pki-ca.crt=/path/to/acme-issuer-ca.crt`,children:(0,s.jsxs)(t.p,{children:["The validated path expects ",(0,s.jsx)(t.code,{children:"token"}),", ",(0,s.jsx)(t.code,{children:"ca.crt"}),", and ",(0,s.jsx)(t.code,{children:"pki-ca.crt"})," in the same Secret so Transit and ACME trust material stay aligned."]})}),"\n",(0,s.jsx)(t.h2,{id:"step-3-expose-the-acme-challenge-service-with-passthrough",children:"Step 3: Expose the ACME challenge Service with passthrough"}),"\n",(0,s.jsx)(i,{language:"yaml",label:"apply",title:"Create the user-managed passthrough route",code:`apiVersion: traefik.io/v1alpha1
kind: IngressRouteTCP
metadata:
name: bao-acme
namespace: <namespace>
spec:
entryPoints:
  - websecure
routes:
  - match: HostSNI(\`<external-host>\`)
    services:
      - name: <cluster-name>-acme
        port: 443
tls:
  passthrough: true`,children:(0,s.jsxs)(t.p,{children:[(0,s.jsx)(t.code,{children:"tls.mode: ACME"})," requires passthrough. If the edge terminates TLS first, OpenBao cannot complete ACME challenges."]})}),"\n",(0,s.jsx)(t.h2,{id:"step-4-apply-the-hardened-acme-cluster",children:"Step 4: Apply the hardened ACME cluster"}),"\n",(0,s.jsx)(i,{language:"yaml",label:"apply",title:"Apply the validated hardened ACME manifest",code:`apiVersion: openbao.org/v1alpha1
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
  mode: ACME
  acme:
    directoryURL: "<acme-directory-url>"
    domains:
      - "<cluster-name>-acme.<namespace>.svc"
      - "<external-host>"
    email: "admin@example.invalid"

configuration:
  logLevel: "info"
  ui: true
  logging:
    format: "json"
  acmeCARoot: "/etc/bao/seal-creds/ca.crt"

unseal:
  type: transit
  credentialsSecretRef:
    name: trust-services-token
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
  strategy: RollingUpdate`}),"\n",(0,s.jsx)(a,{type:"note",title:"Internal `.svc` domain",children:(0,s.jsxs)(t.p,{children:["The internal ",(0,s.jsx)(t.code,{children:".svc"})," hostname in ",(0,s.jsx)(t.code,{children:"spec.tls.acme.domains"})," gives the local lane a stable service name for certificate issuance and cluster-internal joins while the external hostname stays present in the SAN set."]})}),"\n",(0,s.jsxs)(a,{type:"note",title:"AppArmor on local clusters",children:[(0,s.jsx)(t.p,{children:"If kubelet rejects the Pods because AppArmor is unavailable, add:"}),(0,s.jsx)(t.pre,{children:(0,s.jsx)(t.code,{className:"language-yaml",children:"spec:\n  workloadHardening:\n    appArmorEnabled: false\n"})})]}),"\n",(0,s.jsx)(t.h2,{id:"verify-the-lane",children:"Verify the lane"}),"\n",(0,s.jsx)(i,{language:"bash",label:"verify",title:"Check the cluster conditions",code:`kubectl -n <namespace> get openbaocluster <cluster-name> \\
-o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'`,children:(0,s.jsxs)(t.p,{children:["The steady-state expectation is ",(0,s.jsx)(t.code,{children:"Available=True"}),", ",(0,s.jsx)(t.code,{children:"ACMEIntegrationReady=True"}),", ",(0,s.jsx)(t.code,{children:"ACMECacheReady=True"}),", ",(0,s.jsx)(t.code,{children:"UserAccessBootstrap=True"}),", ",(0,s.jsx)(t.code,{children:"ProductionReady=True"}),", ",(0,s.jsx)(t.code,{children:"OpenBaoInitialized=True"}),", and ",(0,s.jsx)(t.code,{children:"OpenBaoSealed=False"}),"."]})}),"\n",(0,s.jsx)(i,{language:"bash",label:"verify",title:"Verify that the ACME Service exists and no external TLS Secret is required",code:`kubectl -n <namespace> get svc <cluster-name>-acme
kubectl -n <namespace> get secret <cluster-name>-tls-server`,children:(0,s.jsxs)(t.p,{children:["The service should exist. The Secret lookup should return ",(0,s.jsx)(t.code,{children:"NotFound"}),", because OpenBao manages the leaf certificate itself in this lane."]})}),"\n",(0,s.jsx)(i,{language:"bash",label:"verify",title:"Verify JWT admin login",code:`kubectl -n <namespace> port-forward svc/<cluster-name> 8200:8200
export VAULT_ADDR="https://127.0.0.1:8200"
JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \\
-H 'Content-Type: application/json' \\
-d "{\\"role\\":\\"admin\\",\\"jwt\\":\\"\${JWT}\\"}" \\
\${VAULT_ADDR%/}/v1/auth/jwt/login`}),"\n",(0,s.jsx)(l,{title:"Keep moving",items:[{label:"Reference architecture",description:"Review the lane summary, topology, and invariants behind this local ACME path.",docId:"user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme"},{label:"Troubleshoot the cluster",description:"Use the incident routing page if ACME domain reachability or certificate readiness does not settle.",docId:"user-guide/openbaocluster/operations/troubleshooting"},{label:"EKS Hardened",description:"Move to the cloud baseline when you need public ACME, KMS auto-unseal, and a dedicated passthrough Gateway.",docId:"user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme"}]})]})}function p(e={}){let{wrapper:t}={...(0,r.R)(),...e.components};return t?(0,s.jsx)(t,{...e,children:(0,s.jsx)(d,{...e})}):d(e)}function h(e,t){throw Error("Expected "+(t?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},10506(e,t,a){a.d(t,{R:()=>i,x:()=>o});var n=a(12888);let s={},r=n.createContext(s);function i(e){let t=n.useContext(r);return n.useMemo(function(){return"function"==typeof e?e(t):{...t,...e}},[t,e])}function o(e){let t;return t=e.disableParentContext?"function"==typeof e.components?e.components(s):e.components||s:i(e.components),n.createElement(r.Provider,{value:t},e.children)}}}]);
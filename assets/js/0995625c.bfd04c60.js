"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["5993"],{72858(e,t,o){o.r(t),o.d(t,{metadata:()=>a,default:()=>d,frontMatter:()=>r,contentTitle:()=>n,toc:()=>c,assets:()=>l});var a=JSON.parse('{"id":"user-guide/openbaocluster/configuration/self-init","title":"Self-Initialization","description":"Configure bootstrap requests, operator OIDC setup, and verification for self-initializing OpenBao clusters without leaving a persistent root token behind.","source":"@site/versioned_docs/version-0.1.0/user-guide/openbaocluster/configuration/self-init.md","sourceDirName":"user-guide/openbaocluster/configuration","slug":"/user-guide/openbaocluster/configuration/self-init","permalink":"/openbao-operator/docs/user-guide/openbaocluster/configuration/self-init","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/openbaocluster/configuration/self-init.md","tags":[],"version":"0.1.0","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1774904343000,"frontMatter":{"title":"Self-Initialization","hide_title":true,"pageType":"task","journey":"configure","description":"Configure bootstrap requests, operator OIDC setup, and verification for self-initializing OpenBao clusters without leaving a persistent root token behind."},"sidebar":"operatorDocs","previous":{"title":"Security profiles","permalink":"/openbao-operator/docs/configure/security-profiles"},"next":{"title":"Unseal configuration","permalink":"/openbao-operator/docs/configure/unseal"}}'),s=o(74848),i=o(28453);let r={title:"Self-Initialization",hide_title:!0,pageType:"task",journey:"configure",description:"Configure bootstrap requests, operator OIDC setup, and verification for self-initializing OpenBao clusters without leaving a persistent root token behind."},n,l={},c=[{value:"Enable self-init",id:"enable-self-init",level:2},{value:"What belongs in <code>requests</code>",id:"what-belongs-in-requests",level:2},{value:"Bootstrap operator OIDC roles",id:"bootstrap-operator-oidc-roles",level:2},{value:"Common bootstrap patterns",id:"common-bootstrap-patterns",level:2},{value:"Verify the cluster finished bootstrap",id:"verify-the-cluster-finished-bootstrap",level:2}];function u(e){let t={a:"a",code:"code",h2:"h2",p:"p",...(0,i.R)(),...e.components},{Callout:o,CommandBlock:a,DecisionTable:r,DiagramFrame:n,NextActions:l,PageHeader:c,TabItem:u,Tabs:d}=t;return o||p("Callout",!0),a||p("CommandBlock",!0),r||p("DecisionTable",!0),n||p("DiagramFrame",!0),l||p("NextActions",!0),c||p("PageHeader",!0),u||p("TabItem",!0),d||p("Tabs",!0),(0,s.jsxs)(s.Fragment,{children:[(0,s.jsx)(c,{title:"Bootstrap the cluster declaratively and avoid carrying a root token forward.",lede:"Self-initialization lets the cluster bring up auth methods, policies, audit devices, and other bootstrap state as part of the `OpenBaoCluster` manifest. It is the supported production bootstrap path because it avoids leaving a long-lived root token in a Kubernetes Secret."}),"\n",(0,s.jsx)(r,{title:"Choose the bootstrap path deliberately",columns:["Path","Use it when","What happens to the root token","Watch for"],rows:[{cells:["Self-initialization","You want declarative bootstrap and a production-ready baseline.","The root token is auto-revoked after the requests complete successfully.","You must define at least one usable auth path for humans or automation before the cluster comes up."],emphasis:"recommended"},{cells:["Standard init","You need a temporary compatibility path for development or controlled manual setup.","A root token can be created and stored in a Secret.","This is easier to start with, but it leaves you with a stronger credential-management burden afterward."],emphasis:"caution"}]}),"\n",(0,s.jsx)(n,{title:"Self-init bootstrap flow",caption:"The cluster initializes, applies the declared requests, and then revokes the bootstrap credential instead of treating it as a permanent operating dependency.",code:`flowchart LR
  Cluster["OpenBaoCluster"] --> Init["Cluster initializes"]
  Init --> Requests["Apply selfInit requests"]
  Requests --> Auth["Auth methods and policies"]
  Requests --> Audit["Audit devices and engines"]
  Requests --> Revoke["Revoke root token"]
  Revoke --> Ready["Cluster ready for day 2"]

  classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
  classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
  classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

  class Cluster,Init read;
  class Requests,Revoke process;
  class Auth,Audit,Ready write;`}),"\n",(0,s.jsx)(o,{type:"warning",title:"Lockout is the real failure mode",children:(0,s.jsx)(t.p,{children:"Self-init is safer only if you plan the access path up front. If you enable it without creating a usable auth method for operators or humans, the root token is revoked and the cluster can become effectively unreachable without recreation."})}),"\n",(0,s.jsx)(r,{title:"Bootstrap both access surfaces together",columns:["Access surface","Where it lives","What must be true before first reconcile"],rows:[{cells:["Operator lifecycle auth","`spec.selfInit.oidc.enabled`","Enable it when you want the operator to bootstrap JWT auth for backup, restore, and upgrade work."],emphasis:"recommended"},{cells:["Human login path","`spec.selfInit.requests`","Create at least one usable auth method and policy path for people before the root token is revoked."]}]}),"\n",(0,s.jsx)(o,{type:"tip",title:"Self-init is the whole bootstrap contract",children:(0,s.jsxs)(t.p,{children:["Do not think of operator auth as step one and human auth as something to add later.\nIf the cluster will self-initialize, define the human login path in ",(0,s.jsx)(t.code,{children:"selfInit.requests"})," as part of the same manifest that enables self-init."]})}),"\n",(0,s.jsx)(t.h2,{id:"enable-self-init",children:"Enable self-init"}),"\n",(0,s.jsx)(a,{language:"yaml",label:"configure",title:"Start from the minimum self-init block",code:`spec:
selfInit:
  enabled: true
  requests:
    - name: enable-audit
      operation: update
      path: sys/audit/file
      auditDevice:
        type: file
        fileOptions:
          filePath: /tmp/audit.log`,children:(0,s.jsxs)(t.p,{children:["Treat ",(0,s.jsx)(t.code,{children:"requests"})," as part of the bootstrap contract. They should create the minimum auth, policy, and audit state required for the cluster to be useful after bootstrap."]})}),"\n",(0,s.jsx)(a,{language:"yaml",label:"configure",title:"Pair operator OIDC bootstrap with a human auth path",code:`spec:
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
    - name: create-admin-policy
      operation: update
      path: sys/policies/acl/admin
      policy:
        policy: |
          path "*" {
            capabilities = ["create", "read", "update", "delete", "list", "sudo"]
          }
    # Add your user, JWT role, or Kubernetes auth role here so a real human
    # login path exists before the root token is revoked.`,children:(0,s.jsxs)(t.p,{children:["The exact human auth method is your choice, but it belongs inside the same ",(0,s.jsx)(t.code,{children:"selfInit"})," contract. For a complete worked example, see the ",(0,s.jsx)(t.a,{href:"/openbao-operator/docs/user-guide/validated-deployments/recipes/local/development-self-init-userpass",children:"development self-init userpass recipe"}),"."]})}),"\n",(0,s.jsxs)(t.h2,{id:"what-belongs-in-requests",children:["What belongs in ",(0,s.jsx)(t.code,{children:"requests"})]}),"\n",(0,s.jsx)(r,{kind:"reference",title:"Structured request surfaces",columns:["Surface","Use it for","Typical example"],rows:[{cells:["`authMethod`","Enable and configure auth backends.","JWT or Kubernetes auth for operators and clients."],emphasis:"recommended"},{cells:["`policy`","Create ACL policies that your auth methods will bind to.","Policies for apps, operators, or bootstrap-only roles."]},{cells:["`secretEngine`","Enable mounts such as KV or transit.","Initial application secret storage or cryptography services."]},{cells:["`auditDevice`","Turn on audit logging at bootstrap time.","File or stdout audit devices required by your environment."]},{cells:["`data`","Fallback for raw API payloads when no structured field exists.","Specialized configuration that is not covered by the higher-level request fields yet."],emphasis:"caution"}]}),"\n",(0,s.jsx)(o,{type:"danger",title:"Do not embed raw secrets in requests",children:(0,s.jsx)(t.p,{children:"Avoid placing passwords, tokens, or key material directly in the manifest. Use Kubernetes Secrets where supported and treat bootstrap content like the rest of your GitOps security surface."})}),"\n",(0,s.jsx)(t.h2,{id:"bootstrap-operator-oidc-roles",children:"Bootstrap operator OIDC roles"}),"\n",(0,s.jsx)(a,{language:"yaml",label:"configure",title:"Enable operator OIDC bootstrap",code:`spec:
selfInit:
  enabled: true
  oidc:
    enabled: true
    # Optional:
    # issuer: "https://..."
    # audience: "openbao-internal"`,children:(0,s.jsx)(t.p,{children:"This bootstraps operator-only JWT auth roles for lifecycle work such as backup, upgrade, and restore. It does not create human login paths by itself."})}),"\n",(0,s.jsx)(r,{kind:"reference",title:"What must stay aligned",columns:["Surface","Why it matters","What to align"],rows:[{cells:["OIDC issuer and JWKS discovery","The operator must discover the Kubernetes issuer and keys to bootstrap JWT auth cleanly.","Ensure the operator ServiceAccount can GET the OIDC discovery and JWKS non-resource URLs."],emphasis:"recommended"},{cells:["JWT audience","The role binding inside OpenBao and the projected token audience must match.","Keep `spec.selfInit.oidc.audience` aligned with the installation-scoped `OPENBAO_JWT_AUDIENCE` value."]},{cells:["Rendered controller identity","Custom namespace or name-prefix changes affect the ServiceAccount subject the JWT role expects.","If you manage roles manually, bind them to the rendered controller ServiceAccount identity rather than to a guessed default."]}]}),"\n",(0,s.jsx)(t.h2,{id:"common-bootstrap-patterns",children:"Common bootstrap patterns"}),"\n",(0,s.jsxs)(d,{groupId:"self-init-patterns",children:[(0,s.jsx)(u,{value:"auth-method",label:"Auth method",children:(0,s.jsx)(a,{language:"yaml",label:"configure",title:"Enable a JWT auth method",code:`- name: enable-jwt
operation: update
path: sys/auth/jwt-operator
authMethod:
  type: jwt
  description: "Kubernetes JWT auth"
  config:
    default_lease_ttl: "1h"
    max_lease_ttl: "24h"`})}),(0,s.jsx)(u,{value:"policy",label:"Policy",children:(0,s.jsx)(a,{language:"yaml",label:"configure",title:"Create an ACL policy at bootstrap",code:`- name: app-policy
operation: update
path: sys/policies/acl/app-policy
policy:
  policy: |
    path "secret/data/app/*" {
      capabilities = ["read", "list"]
    }`})}),(0,s.jsx)(u,{value:"secret-engine",label:"Secret engine",children:(0,s.jsx)(a,{language:"yaml",label:"configure",title:"Enable a KV v2 mount",code:`- name: enable-kv-v2
operation: update
path: sys/mounts/secret
secretEngine:
  type: kv
  description: "General purpose KV store"
  options:
    version: "2"`})}),(0,s.jsx)(u,{value:"audit-device",label:"Audit device",children:(0,s.jsx)(a,{language:"yaml",label:"configure",title:"Enable audit logging at bootstrap",code:`- name: enable-file-audit
operation: update
path: sys/audit/file
auditDevice:
  type: file
  fileOptions:
    filePath: /var/log/openbao/audit.log`})})]}),"\n",(0,s.jsx)(t.h2,{id:"verify-the-cluster-finished-bootstrap",children:"Verify the cluster finished bootstrap"}),"\n",(0,s.jsx)(a,{language:"bash",label:"verify",title:"Check the self-init status bit",code:"kubectl get openbaocluster <name> -o jsonpath='{.status.selfInitialized}'",children:(0,s.jsxs)(t.p,{children:["A healthy bootstrap should report ",(0,s.jsx)(t.code,{children:"true"}),". If it does not, inspect the cluster status conditions and controller logs before retrying with additional requests."]})}),"\n",(0,s.jsx)(l,{title:"Continue cluster baseline",items:[{label:"Server configuration",description:"Move from bootstrap into the steady-state server settings and autopilot defaults you want to keep.",docId:"user-guide/openbaocluster/configuration/server"},{label:"Operator authentication",description:"Review the operator-side auth contract if you are relying on OIDC bootstrap.",docId:"user-guide/operator/authn"},{label:"Backup operations",description:"See how the operator OIDC role is used by later lifecycle workflows.",docId:"user-guide/openbaocluster/operations/backups"}]})]})}function d(e={}){let{wrapper:t}={...(0,i.R)(),...e.components};return t?(0,s.jsx)(t,{...e,children:(0,s.jsx)(u,{...e})}):u(e)}function p(e,t){throw Error("Expected "+(t?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},28453(e,t,o){o.d(t,{R:()=>r,x:()=>n});var a=o(96540);let s={},i=a.createContext(s);function r(e){let t=a.useContext(i);return a.useMemo(function(){return"function"==typeof e?e(t):{...t,...e}},[t,e])}function n(e){let t;return t=e.disableParentContext?"function"==typeof e.components?e.components(s):e.components||s:r(e.components),a.createElement(i.Provider,{value:t},e.children)}}}]);
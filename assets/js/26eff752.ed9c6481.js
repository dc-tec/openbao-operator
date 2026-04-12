"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["6431"],{41400(e,t,o){o.r(t),o.d(t,{metadata:()=>a,default:()=>d,frontMatter:()=>i,contentTitle:()=>s,toc:()=>c,assets:()=>l});var a=JSON.parse('{"id":"user-guide/openbaocluster/configuration/security-profiles","title":"Security Profiles","description":"Choose the cluster posture first, including bootstrap, unseal, TLS, and image-verification expectations for development versus Hardened production.","source":"@site/../docs/user-guide/openbaocluster/configuration/security-profiles.md","sourceDirName":"user-guide/openbaocluster/configuration","slug":"/configure/security-profiles","permalink":"/openbao-operator/docs/next/configure/security-profiles","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/openbaocluster/configuration/security-profiles.md","tags":[],"version":"current","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1774819589000,"frontMatter":{"title":"Security Profiles","slug":"/configure/security-profiles","hide_title":true,"pageType":"task","journey":"configure","description":"Choose the cluster posture first, including bootstrap, unseal, TLS, and image-verification expectations for development versus Hardened production."},"sidebar":"operatorDocs","previous":{"title":"Cluster overview","permalink":"/openbao-operator/docs/next/configure/cluster-overview"},"next":{"title":"Self-initialization","permalink":"/openbao-operator/docs/next/user-guide/openbaocluster/configuration/self-init"}}'),r=o(74848),n=o(28453);let i={title:"Security Profiles",slug:"/configure/security-profiles",hide_title:!0,pageType:"task",journey:"configure",description:"Choose the cluster posture first, including bootstrap, unseal, TLS, and image-verification expectations for development versus Hardened production."},s,l={},c=[{value:"What actually changes with the profile",id:"what-actually-changes-with-the-profile",level:2},{value:"Representative starting points",id:"representative-starting-points",level:2},{value:"Choose the unseal root of trust",id:"choose-the-unseal-root-of-trust",level:2},{value:"Optional runtime hardening",id:"optional-runtime-hardening",level:2}];function u(e){let t={h2:"h2",p:"p",...(0,n.R)(),...e.components},{Callout:o,CommandBlock:a,DecisionTable:i,DiagramFrame:s,NextActions:l,PageHeader:c,SiteLink:u,TabItem:d,Tabs:h}=t;return o||p("Callout",!0),a||p("CommandBlock",!0),i||p("DecisionTable",!0),s||p("DiagramFrame",!0),l||p("NextActions",!0),c||p("PageHeader",!0),u||p("SiteLink",!0),d||p("TabItem",!0),h||p("Tabs",!0),(0,r.jsxs)(r.Fragment,{children:[(0,r.jsx)(c,{title:"Choose the operating posture before you tune anything else.",lede:"`spec.profile` is the top-level decision that shapes bootstrap, unseal, TLS, image verification, and failure tolerance. Pick the posture you plan to keep operating, then let the rest of the cluster baseline follow from that choice."}),"\n",(0,r.jsx)(i,{title:"Choose the profile deliberately",columns:["Profile","Use it when","What it assumes","Avoid it when"],rows:[{cells:["Hardened","The cluster is intended to become a real production service.","External unseal, self-initialization, verified TLS, and supply-chain guardrails are part of the normal path.","You cannot meet the external trust, identity, or networking requirements yet."],emphasis:"recommended"},{cells:["Development","You need a fast local or evaluation path and accept weaker security defaults temporarily.","Bootstrap material may live in Kubernetes Secrets, TLS can be operator-managed, and verification can be relaxed.","You are trying to define the baseline that will survive into production."],emphasis:"caution"}]}),"\n",(0,r.jsx)(s,{title:"How the profile shapes the baseline",caption:"The profile is not cosmetic. It determines whether the cluster can rely on operator-generated trust material and stored bootstrap credentials or whether those paths are explicitly disallowed.",code:`flowchart LR
  Profile["spec.profile"] --> Bootstrap["Bootstrap path"]
  Profile --> Unseal["Unseal root of trust"]
  Profile --> TLS["TLS source"]
  Profile --> Verify["Image verification"]
  Profile --> Risk["Status and admission guardrails"]

  Bootstrap --> SelfInit["Self-initialization or standard init"]
  Unseal --> KMS["External KMS / transit / static"]
  TLS --> External["External / ACME / operator-managed"]
  Verify --> Policy["Block or warn"]
  Risk --> Conditions["Conditions and validation policy"]

  classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
  classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
  classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

  class Profile process;
  class Bootstrap,Unseal,TLS,Verify,Risk read;
  class SelfInit,KMS,External,Policy,Conditions write;`}),"\n",(0,r.jsx)(t.h2,{id:"what-actually-changes-with-the-profile",children:"What actually changes with the profile"}),"\n",(0,r.jsx)(i,{kind:"reference",title:"Profile effects",columns:["Surface","Development","Hardened"],rows:[{cells:["Bootstrap credential handling","Manual init is allowed and can leave a root token in a Secret when self-init is disabled.","Self-initialization is the supported path and root-token persistence is not the normal operating model."],emphasis:"recommended"},{cells:["Unseal","Static unseal in a Secret is allowed for fast evaluation.","Use an external trust source such as cloud KMS or transit. Treat static unseal as a non-production exception."]},{cells:["TLS","Operator-managed TLS is acceptable for development and internal evaluation.","Use `External` or `ACME`; the certificate authority should not be operator-generated in production."]},{cells:["Image verification","Can be introduced gradually and warning-only rollouts are possible.","Verification is expected, warning-only behavior is not the production posture, and official-image defaults still verify when trust material is omitted."]},{cells:["Networking and jobs","You can tolerate more permissive local defaults while standing up the cluster.","Backup and other lifecycle paths should assume explicit egress and identity wiring before go-live."]}]}),"\n",(0,r.jsx)(t.h2,{id:"representative-starting-points",children:"Representative starting points"}),"\n",(0,r.jsxs)(h,{groupId:"configure-profile-hardened-development",children:[(0,r.jsx)(d,{value:"hardened",label:"Hardened",children:(0,r.jsx)(a,{language:"yaml",label:"configure",title:"Start from the supported production baseline",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: prod-cluster
spec:
profile: Hardened
replicas: 3
version: "2.4.4"
selfInit:
  enabled: true
tls:
  enabled: true
  mode: External
unseal:
  type: awskms
  awskms:
    region: us-east-1
    kmsKeyID: alias/openbao-unseal
imageVerification:
  enabled: true
  failurePolicy: Block
operatorImageVerification:
  enabled: true
  failurePolicy: Block`,children:(0,r.jsx)(t.p,{children:"Hardened is the supported production path. It assumes an external trust source for unseal, non-operator-managed TLS, and a self-initializing bootstrap flow."})})}),(0,r.jsx)(d,{value:"development",label:"Development",children:(0,r.jsx)(a,{language:"yaml",label:"configure",title:"Use the lightest safe evaluation baseline",code:`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
name: dev-cluster
spec:
profile: Development
version: "2.4.4"
replicas: 1
tls:
  enabled: true
unseal:
  type: static`,children:(0,r.jsx)(t.p,{children:"Development is for local testing, proof-of-concept work, and environments where you explicitly accept stored bootstrap material and weaker trust boundaries."})})})]}),"\n",(0,r.jsx)(o,{type:"warning",title:"Do not let development defaults drift into production",children:(0,r.jsx)(t.p,{children:"The dangerous part of the Development profile is not that it exists; it is that it feels easy to keep. If the cluster matters, switch to Hardened before other systems begin to depend on it."})}),"\n",(0,r.jsx)(t.h2,{id:"choose-the-unseal-root-of-trust",children:"Choose the unseal root of trust"}),"\n",(0,r.jsx)(i,{title:"Unseal options by posture",columns:["Path","Use it when","Why it fits or does not fit"],rows:[{cells:["Cloud KMS","You run in AWS, GCP, Azure, or another managed platform with a usable external key service.","This is usually the cleanest Hardened path because the root of trust stays outside Kubernetes."],emphasis:"recommended"},{cells:["Transit","You already run a central OpenBao cluster or equivalent external trust service.","This works well for multi-cluster or hybrid environments where central trust management is intentional."]},{cells:["PKCS#11 or KMIP","You need HSM-backed or enterprise key-management integration.","Valid for production, but usually more specialized and operationally heavier than cloud KMS or transit."]},{cells:["Static Secret","You need a local development path and understand the blast radius.","This is convenient but keeps decryption material inside the same cluster state you are trying to protect."],emphasis:"caution"}]}),"\n",(0,r.jsxs)(h,{groupId:"configure-unseal-common-patterns",children:[(0,r.jsx)(d,{value:"aws-kms",label:"AWS KMS",children:(0,r.jsx)(a,{language:"yaml",label:"configure",title:"Use AWS KMS for Hardened unseal",code:`spec:
profile: Hardened
serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: "arn:aws:iam::123456789012:role/openbao-awskms"
unseal:
  type: awskms
  awskms:
    kmsKeyID: "arn:aws:kms:us-east-1:123456789012:key/..."
    region: "us-east-1"`})}),(0,r.jsx)(d,{value:"gcp-kms",label:"GCP KMS",children:(0,r.jsx)(a,{language:"yaml",label:"configure",title:"Use GCP Cloud KMS for Hardened unseal",code:`spec:
profile: Hardened
serviceAccount:
  annotations:
    iam.gke.io/gcp-service-account: "openbao@my-project.iam.gserviceaccount.com"
unseal:
  type: gcpckms
  gcpCloudKMS:
    project: "my-project"
    region: "us-central1"
    keyRing: "openbao-ring"
    cryptoKey: "openbao-key"`})}),(0,r.jsx)(d,{value:"azure-kv",label:"Azure Key Vault",children:(0,r.jsx)(a,{language:"yaml",label:"configure",title:"Use Azure Key Vault for Hardened unseal",code:`spec:
profile: Hardened
serviceAccount:
  annotations:
    azure.workload.identity/client-id: "87654321-4321-4321-4321-210987654321"
podMetadata:
  labels:
    azure.workload.identity/use: "true"
unseal:
  type: azurekeyvault
  azureKeyVault:
    vaultName: "my-vault"
    keyName: "openbao-key"`})}),(0,r.jsx)(d,{value:"transit",label:"Transit",children:(0,r.jsx)(a,{language:"yaml",label:"configure",title:"Use a central OpenBao transit key for unseal",code:`spec:
profile: Hardened
unseal:
  type: transit
  credentialsSecretRef:
    name: transit-unseal-creds
  transit:
    address: "https://central-openbao.example.com"
    keyName: "tenant-1-key"
    mountPath: "transit"`,children:(0,r.jsx)(t.p,{children:"The referenced Secret should hold the transit token and any optional CA or client-certificate material required by that upstream cluster."})})})]}),"\n",(0,r.jsx)(t.h2,{id:"optional-runtime-hardening",children:"Optional runtime hardening"}),"\n",(0,r.jsx)(a,{language:"yaml",label:"configure",title:"Enable AppArmor when the nodes support it",code:`spec:
workloadHardening:
  appArmorEnabled: true`,children:(0,r.jsxs)(t.p,{children:["AppArmor is opt-in because support depends on the underlying node OS and cluster runtime. Pair this with the broader workload baseline in ",(0,r.jsx)(u,{docId:"security/workload/workload-security",children:"Pod and runtime security"}),"."]})}),"\n",(0,r.jsx)(l,{title:"Continue cluster baseline",items:[{label:"Unseal configuration",description:"Use the provider-by-provider Secret and mounted-file contract page once you know which root of trust you want.",docId:"user-guide/openbaocluster/configuration/unseal"},{label:"Self-initialization",description:"Configure the bootstrap requests and operator OIDC flow that follow from the profile choice.",docId:"user-guide/openbaocluster/configuration/self-init"},{label:"External access",description:"Choose the TLS and exposure pattern that matches the baseline you just picked.",docId:"user-guide/openbaocluster/configuration/external-access"},{label:"Workload protections",description:"Review the runtime and supply-chain controls expected behind the Hardened posture.",docId:"security/workload/index"}]})]})}function d(e={}){let{wrapper:t}={...(0,n.R)(),...e.components};return t?(0,r.jsx)(t,{...e,children:(0,r.jsx)(u,{...e})}):u(e)}function p(e,t){throw Error("Expected "+(t?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},28453(e,t,o){o.d(t,{R:()=>i,x:()=>s});var a=o(96540);let r={},n=a.createContext(r);function i(e){let t=a.useContext(n);return a.useMemo(function(){return"function"==typeof e?e(t):{...t,...e}},[t,e])}function s(e){let t;return t=e.disableParentContext?"function"==typeof e.components?e.components(r):e.components||r:i(e.components),a.createElement(n.Provider,{value:t},e.children)}}}]);
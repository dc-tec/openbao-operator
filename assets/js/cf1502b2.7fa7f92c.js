"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["2165"],{92427(e,t,r){r.r(t),r.d(t,{metadata:()=>s,default:()=>p,frontMatter:()=>a,contentTitle:()=>i,toc:()=>c,assets:()=>l});var s=JSON.parse('{"id":"user-guide/openbaocluster/configuration/network","title":"Network Configuration","description":"Configure the operator-managed NetworkPolicy contract, including DNS, Kubernetes API egress, trusted ingress peers, and external dependency paths for backups and restore.","source":"@site/versioned_docs/version-0.3.0/user-guide/openbaocluster/configuration/network.md","sourceDirName":"user-guide/openbaocluster/configuration","slug":"/user-guide/openbaocluster/configuration/network","permalink":"/openbao-operator/docs/user-guide/openbaocluster/configuration/network","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/openbaocluster/configuration/network.md","tags":[],"version":"0.3.0","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1780355522000,"frontMatter":{"title":"Network Configuration","hide_title":true,"pageType":"task","journey":"configure","description":"Configure the operator-managed NetworkPolicy contract, including DNS, Kubernetes API egress, trusted ingress peers, and external dependency paths for backups and restore."},"sidebar":"operatorDocs","previous":{"title":"External access","permalink":"/openbao-operator/docs/configure/external-access"},"next":{"title":"Gateway API support","permalink":"/openbao-operator/docs/user-guide/openbaocluster/configuration/gateway-api"}}'),o=r(74848),n=r(28453);let a={title:"Network Configuration",hide_title:!0,pageType:"task",journey:"configure",description:"Configure the operator-managed NetworkPolicy contract, including DNS, Kubernetes API egress, trusted ingress peers, and external dependency paths for backups and restore."},i,l={},c=[{value:"DNS and Kubernetes API egress",id:"dns-and-kubernetes-api-egress",level:2},{value:"Add the environment-specific paths",id:"add-the-environment-specific-paths",level:2},{value:"Read the operator conditions",id:"read-the-operator-conditions",level:2}];function d(e){let t={code:"code",h2:"h2",p:"p",...(0,n.R)(),...e.components},{CommandBlock:r,DecisionTable:s,DiagramFrame:a,NextActions:i,PageHeader:l,TabItem:c,Tabs:d}=t;return r||u("CommandBlock",!0),s||u("DecisionTable",!0),a||u("DiagramFrame",!0),i||u("NextActions",!0),l||u("PageHeader",!0),c||u("TabItem",!0),d||u("Tabs",!0),(0,o.jsxs)(o.Fragment,{children:[(0,o.jsx)(l,{title:"Network policy and traffic contracts",lede:"The operator starts from a deny-by-default posture and then adds the ingress and egress paths the cluster needs to function. Use this page to configure DNS, Kubernetes API access, edge peers, and external dependency traffic for backup and restore workflows."}),"\n",(0,o.jsx)(a,{title:"Default network posture",caption:"The namespace starts from deny-by-default, then allows the operator, peer traffic, DNS, Kubernetes API access, and whichever external systems you configure deliberately.",code:`flowchart TB
  subgraph External ["External systems"]
      Edge["Gateway / ingress"]
      Storage["Backup or restore storage"]
      Transit["Transit / KMS / PKI"]
  end

  subgraph Cluster ["Kubernetes cluster"]
      API["Kubernetes API"]
      DNS["DNS"]
      Operator["Operator"]
      Bao["OpenBao Pods"]
      Jobs["Backup / restore Jobs"]
  end

  Edge --> Bao
  Operator --> Bao
  Bao --> Bao
  Bao --> DNS
  Bao --> API
  Jobs --> DNS
  Jobs --> Storage
  Bao --> Transit

  classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
  classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
  classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

  class API,DNS,Storage,Transit read;
  class Operator,Edge process;
  class Bao,Jobs write;`}),"\n",(0,o.jsx)(s,{title:"Traffic the operator expects by default",columns:["Direction","Path","Why it exists"],rows:[{cells:["Ingress","Operator to OpenBao on the service listener","Health checks, initialization, unseal coordination, and lifecycle orchestration all depend on this control-plane path."],emphasis:"recommended"},{cells:["Ingress","OpenBao peer-to-peer traffic","Raft members need to exchange cluster traffic on the peer port."]},{cells:["Egress","DNS and Kubernetes API","Pods and Jobs need name resolution and selected Kubernetes API access under strict policy."]},{cells:["Conditional ingress or egress","Gateway, ingress-controller, storage, transit, or PKI paths","These are environment-specific and should be configured explicitly rather than allowed broadly."]}]}),"\n",(0,o.jsx)(t.h2,{id:"dns-and-kubernetes-api-egress",children:"DNS and Kubernetes API egress"}),"\n",(0,o.jsx)(s,{kind:"reference",title:"Core network settings",columns:["Field","Use it for","When it matters"],rows:[{cells:["`network.dnsNamespace`","Tell the operator where your DNS service actually runs.","Use this when the cluster DNS namespace is not `kube-system`, such as on OpenShift."],emphasis:"recommended"},{cells:["`network.dnsEndpointIPs`","Allow direct DNS egress to resolver IPs instead of only to pod-backed Services.","Use this for node-local caches or host-networked DNS topologies where service-based rules are insufficient."]},{cells:["`network.apiServerCIDR`","Override the default service-VIP allow-list for Kubernetes API access.","Use this when you know the exact API-service CIDR you want to allow."]},{cells:["`network.apiServerEndpointIPs`","Allow egress directly to backing API-server endpoint IPs.","Use this when your CNI evaluates policy post-DNAT and the service VIP alone is not enough."]}]}),"\n",(0,o.jsxs)(d,{groupId:"configure-network-core",children:[(0,o.jsx)(c,{value:"dns",label:"DNS",children:(0,o.jsx)(r,{language:"yaml",label:"configure",title:"Configure DNS for non-default or node-local resolver paths",code:`spec:
network:
  dnsNamespace: "openshift-dns"
  dnsEndpointIPs:
    - "169.254.20.10"`,children:(0,o.jsxs)(t.p,{children:["Set ",(0,o.jsx)(t.code,{children:"dnsEndpointIPs"})," when the resolver is enforced by IP rather than by Service-backed pod traffic. This setting also applies to backup and restore Jobs."]})})}),(0,o.jsx)(c,{value:"api",label:"Kubernetes API",children:(0,o.jsx)(r,{language:"yaml",label:"configure",title:"Pin Kubernetes API egress explicitly when needed",code:`spec:
network:
  apiServerCIDR: "10.43.0.1/32"
  apiServerEndpointIPs:
    - "192.168.166.2"`,children:(0,o.jsx)(t.p,{children:"Prefer the smallest safe scope. Endpoint IPs are usually only needed when the NetworkPolicy implementation evaluates the post-DNAT destination instead of the service VIP."})})})]}),"\n",(0,o.jsx)(t.h2,{id:"add-the-environment-specific-paths",children:"Add the environment-specific paths"}),"\n",(0,o.jsxs)(d,{groupId:"configure-network-extra-paths",children:[(0,o.jsx)(c,{value:"trusted-ingress",label:"Trusted ingress peers",children:(0,o.jsx)(r,{language:"yaml",label:"configure",title:"Allow an ingress controller or Gateway data plane to reach the cluster",code:`spec:
network:
  trustedIngressPeers:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: traefik
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: ingress-system
      podSelector:
        matchLabels:
          app.kubernetes.io/name: traefik`,children:(0,o.jsxs)(t.p,{children:["Use this when the source is a user-managed ingress controller or Gateway data plane. It is usually clearer than writing a raw ",(0,o.jsx)(t.code,{children:"ingressRules"})," block for the same case. Managed ",(0,o.jsx)(t.code,{children:"spec.ingress.enabled: true"})," configurations require at least one trusted ingress peer."]})})}),(0,o.jsx)(c,{value:"egress",label:"External egress",children:(0,o.jsx)(r,{language:"yaml",label:"configure",title:"Allow egress to transit, storage, or other external systems",code:`spec:
network:
  egressRules:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: openbao-infra
      ports:
        - protocol: TCP
          port: 8200
    - to:
        - ipBlock:
            cidr: 192.168.100.0/24
      ports:
        - protocol: TCP
          port: 443`,children:(0,o.jsx)(t.p,{children:"Use this for transit unseal, object storage, private PKI, or any other external dependency that should not be reachable through a broad allow-all rule."})})}),(0,o.jsx)(c,{value:"ingress",label:"Raw ingress rules",children:(0,o.jsx)(r,{language:"yaml",label:"configure",title:"Add a raw ingress rule when you need full control",code:`spec:
network:
  ingressRules:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
      ports:
        - protocol: TCP
          port: 8200`,children:(0,o.jsxs)(t.p,{children:["Reach for raw ",(0,o.jsx)(t.code,{children:"ingressRules"})," when the source is not a normal ingress-controller path and you need exact port or peer matching."]})})})]}),"\n",(0,o.jsx)(t.h2,{id:"read-the-operator-conditions",children:"Read the operator conditions"}),"\n",(0,o.jsx)(s,{kind:"reference",title:"Conditions that matter",columns:["Condition","What it tells you","Typical next move"],rows:[{cells:["`APIServerNetworkReady=False`","The operator could not build a safe Kubernetes API allow-list.","Fix the API CIDR or endpoint IP configuration first."],emphasis:"recommended"},{cells:["`APIServerNetworkReady=Unknown`","The service-VIP path exists, but your environment may still need explicit endpoint IPs.","Check whether your CNI enforces egress post-DNAT and add `apiServerEndpointIPs` if required."]},{cells:["`BackupConfigurationReady=False` or `RestoreConfigurationReady=False` with `NetworkEgressRulesRequired`","The lifecycle Jobs cannot reach the storage target safely under current policy.","Add explicit storage egress rules before relying on backup or restore workflows."]}]}),"\n",(0,o.jsx)(i,{title:"Continue service boundary setup",items:[{label:"Gateway API support",description:"Use the detailed Gateway API guide when that is the primary edge model.",docId:"user-guide/openbaocluster/configuration/gateway-api"},{label:"Backup operations",description:"Review the object-storage and job-identity path that depends on these network rules.",docId:"user-guide/openbaocluster/operations/backups"},{label:"Network security",description:"Go deeper on the security model behind the deny-by-default posture and namespace isolation.",docId:"security/infrastructure/network-security"}]})]})}function p(e={}){let{wrapper:t}={...(0,n.R)(),...e.components};return t?(0,o.jsx)(t,{...e,children:(0,o.jsx)(d,{...e})}):d(e)}function u(e,t){throw Error("Expected "+(t?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},28453(e,t,r){r.d(t,{R:()=>a,x:()=>i});var s=r(96540);let o={},n=s.createContext(o);function a(e){let t=s.useContext(n);return s.useMemo(function(){return"function"==typeof e?e(t):{...t,...e}},[t,e])}function i(e){let t;return t=e.disableParentContext?"function"==typeof e.components?e.components(o):e.components||o:a(e.components),s.createElement(n.Provider,{value:t},e.children)}}}]);
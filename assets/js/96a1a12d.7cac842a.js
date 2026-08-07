"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["30149"],{31443(e,t,o){o.r(t),o.d(t,{metadata:()=>a,default:()=>d,frontMatter:()=>i,contentTitle:()=>n,toc:()=>c,assets:()=>l});var a=JSON.parse('{"id":"user-guide/operator/authz","title":"Operator Authorization","description":"Understand which policies belong to controller, backup, restore, and upgrade work so destructive capabilities stay scoped to the right identities.","source":"@site/versioned_docs/version-0.1.0/user-guide/operator/authz.md","sourceDirName":"user-guide/operator","slug":"/get-started/operator-authorization","permalink":"/openbao-operator/docs/0.1.0/get-started/operator-authorization","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/operator/authz.md","tags":[],"version":"0.1.0","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1774904343000,"frontMatter":{"title":"Operator Authorization","description":"Understand which policies belong to controller, backup, restore, and upgrade work so destructive capabilities stay scoped to the right identities.","slug":"/get-started/operator-authorization","hide_title":true,"pageType":"concept","journey":"get-started"},"sidebar":"operatorDocs","previous":{"title":"Operator authentication","permalink":"/openbao-operator/docs/0.1.0/get-started/operator-authentication"},"next":{"title":"Tenancy & Governance","permalink":"/openbao-operator/docs/0.1.0/tenant-onboarding"}}'),r=o(91987),s=o(67008);let i={title:"Operator Authorization",description:"Understand which policies belong to controller, backup, restore, and upgrade work so destructive capabilities stay scoped to the right identities.",slug:"/get-started/operator-authorization",hide_title:!0,pageType:"concept",journey:"get-started"},n,l={},c=[{value:"Default policy surfaces",id:"default-policy-surfaces",level:2},{value:"Official OpenBao background",id:"official-openbao-background",level:2}];function p(e){let t={a:"a",h2:"h2",li:"li",p:"p",ul:"ul",...(0,s.R)(),...e.components},{Callout:o,CommandBlock:a,DecisionTable:i,DiagramFrame:n,NextActions:l,PageHeader:c,TabItem:p,Tabs:d}=t;return o||u("Callout",!0),a||u("CommandBlock",!0),i||u("DecisionTable",!0),n||u("DiagramFrame",!0),l||u("NextActions",!0),c||u("PageHeader",!0),p||u("TabItem",!0),d||u("Tabs",!0),(0,r.jsxs)(r.Fragment,{children:[(0,r.jsx)(c,{title:"Keep each operator capability on its own policy surface.",lede:"Authentication answers who a workload is. Authorization answers what that workload can do. OpenBao Operator stays safer when controller, backup, restore, and upgrade work authenticate separately and only receive the policies each path actually needs."}),"\n",(0,r.jsx)(n,{title:"Policies stay attached to job-specific identities",caption:"Each operator path maps to its own JWT role and policy set. The controller is not the universal credential for all day 2 work.",code:`graph LR
  subgraph K8s["Kubernetes identities"]
    Controller["Controller SA"]
    Backup["Backup Job SA"]
    Restore["Restore Job SA"]
    Upgrade["Upgrade Job SA"]
  end

  subgraph Bao["OpenBao auth and policy"]
    RoleController["Role: openbao-operator"]
    RoleBackup["Role: openbao-operator-backup"]
    RoleRestore["Role: openbao-operator-restore"]
    RoleUpgrade["Role: openbao-operator-upgrade"]

    PolicyController["Policy: controller maintenance"]
    PolicyBackup["Policy: snapshot read"]
    PolicyRestore["Policy: snapshot-force"]
    PolicyUpgrade["Policy: rolling or blue-green upgrade"]
  end

  Controller --> RoleController --> PolicyController
  Backup --> RoleBackup --> PolicyBackup
  Restore --> RoleRestore --> PolicyRestore
  Upgrade --> RoleUpgrade --> PolicyUpgrade

  classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
  classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
  classDef caution fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;

  class Controller,Backup,Restore,Upgrade read;
  class RoleController,RoleBackup,RoleRestore,RoleUpgrade write;
  class PolicyController,PolicyBackup,PolicyUpgrade write;
  class PolicyRestore caution;`}),"\n",(0,r.jsx)(i,{title:"Keep policies separated by lifecycle capability",columns:["Policy surface","Used by","Typical capabilities","Why it stays separate"],rows:[{cells:["Controller maintenance","The main controller Deployment","`sys/health`, `sys/step-down`, and autopilot configuration reads or updates","This path should stay available for routine reconciliation and maintenance without inheriting destructive restore powers."],emphasis:"recommended"},{cells:["Backup","The generated backup Job","`sys/storage/raft/snapshot` read access","Snapshot reads are narrower than normal controller maintenance and should be easy to reason about independently."]},{cells:["Restore","The generated restore Job","`sys/storage/raft/snapshot-force` update access","Restore can replace the full cluster state and should only exist on the specific workload that performs restore."],emphasis:"caution"},{cells:["Upgrade","The generated upgrade Job","Step-down, autopilot state, snapshot read, and optional peer-management operations for blue-green flows","Upgrade paths often need time-bounded orchestration permissions that should not widen steady-state controller access."]}]}),"\n",(0,r.jsx)(o,{type:"danger",title:"Treat restore as a destructive role",children:(0,r.jsx)(t.p,{children:"The restore capability can replace data, policies, and keys across the cluster.\nDo not bind the restore policy to the controller or to a broad multi-purpose ServiceAccount just because it is convenient during setup."})}),"\n",(0,r.jsx)(t.h2,{id:"default-policy-surfaces",children:"Default policy surfaces"}),"\n",(0,r.jsxs)(d,{groupId:"operator-policy-surfaces",children:[(0,r.jsx)(p,{value:"controller",label:"Controller",children:(0,r.jsx)(a,{language:"hcl",label:"configure",title:"Controller maintenance policy",code:`path "sys/health" {
capabilities = ["read"]
}

path "sys/step-down" {
capabilities = ["sudo", "update"]
}

path "sys/storage/raft/autopilot/configuration" {
capabilities = ["read", "update"]
}`,children:(0,r.jsx)(t.p,{children:"This is the steady-state controller scope. It should not expand to cover backup, restore, or blue-green peer management unless you are intentionally breaking the model."})})}),(0,r.jsx)(p,{value:"backup",label:"Backup",children:(0,r.jsx)(a,{language:"hcl",label:"configure",title:"Backup policy",code:`path "sys/storage/raft/snapshot" {
capabilities = ["read"]
}`,children:(0,r.jsx)(t.p,{children:"Backup only needs snapshot streaming. Storage-provider credentials are a separate boundary outside this OpenBao policy."})})}),(0,r.jsx)(p,{value:"restore",label:"Restore",children:(0,r.jsx)(a,{language:"hcl",label:"configure",title:"Restore policy",code:`path "sys/storage/raft/snapshot-force" {
capabilities = ["update"]
}`,children:(0,r.jsx)(t.p,{children:"Keep this policy tightly bound to the generated restore Job identity and only for the period where restore is actually needed."})})}),(0,r.jsx)(p,{value:"upgrade",label:"Upgrade",children:(0,r.jsx)(a,{language:"hcl",label:"configure",title:"Rolling and blue-green upgrade policy surfaces",code:`# Rolling upgrade baseline
path "sys/health" {
capabilities = ["read"]
}

path "sys/step-down" {
capabilities = ["sudo", "update"]
}

path "sys/storage/raft/autopilot/state" {
capabilities = ["read"]
}

# Blue-green adds peer-management paths
path "sys/storage/raft/join" {
capabilities = ["update"]
}

path "sys/storage/raft/configuration" {
capabilities = ["read", "update"]
}

path "sys/storage/raft/remove-peer" {
capabilities = ["update"]
}

path "sys/storage/raft/promote" {
capabilities = ["update"]
}

path "sys/storage/raft/demote" {
capabilities = ["update"]
}`,children:(0,r.jsx)(t.p,{children:"Rolling upgrades need less authority than blue-green cutovers. Keep those strategies separate in your head when you review the required policy surface."})})})]}),"\n",(0,r.jsx)(i,{kind:"reference",title:"Common authorization failures",columns:["Symptom","Likely boundary","Check first"],rows:[{cells:["JWT login succeeds but the request returns `permission denied`","The workload authenticated correctly but the policy is missing the needed path capability","Which job or controller path is making the request, then the matching policy surface"],emphasis:"recommended"},{cells:["Backup works but restore fails","The restore Job identity or restore policy is missing or misbound","Restore ServiceAccount, restore role binding, and `snapshot-force` policy"]},{cells:["Rolling upgrade works but blue-green cutover stalls","Peer-management permissions were not added for the upgrade strategy in use","Upgrade strategy and the corresponding upgrade policy paths"]},{cells:["Controller can do too much","A shortcut merged job-specific capabilities into the controller role","Manual auth configuration drift from the intended separation model"]}]}),"\n",(0,r.jsx)(l,{title:"Go deeper",items:[{label:"Operator authentication",description:"Return to the JWT audience and bound-subject model when auth fails before policy even matters.",docId:"user-guide/operator/authn"},{label:"Backup operations",description:"See how the backup Job uses its own auth and storage credentials during normal operation.",docId:"user-guide/openbaocluster/operations/backups"},{label:"Restore manager architecture",description:"Review why restore stays isolated from the controller and how the operator drives it.",docId:"architecture/restore-manager"}]}),"\n",(0,r.jsx)(t.h2,{id:"official-openbao-background",children:"Official OpenBao background"}),"\n",(0,r.jsxs)(t.ul,{children:["\n",(0,r.jsx)(t.li,{children:(0,r.jsx)(t.a,{href:"https://openbao.org/docs/concepts/policies/",children:"Policy concepts"})}),"\n",(0,r.jsx)(t.li,{children:(0,r.jsx)(t.a,{href:"https://openbao.org/docs/commands/policy/",children:"Policy command reference"})}),"\n",(0,r.jsx)(t.li,{children:(0,r.jsx)(t.a,{href:"https://openbao.org/docs/concepts/tokens/",children:"Token concepts"})}),"\n"]})]})}function d(e={}){let{wrapper:t}={...(0,s.R)(),...e.components};return t?(0,r.jsx)(t,{...e,children:(0,r.jsx)(p,{...e})}):p(e)}function u(e,t){throw Error("Expected "+(t?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},67008(e,t,o){o.d(t,{R:()=>i,x:()=>n});var a=o(71763);let r={},s=a.createContext(r);function i(e){let t=a.useContext(s);return a.useMemo(function(){return"function"==typeof e?e(t):{...t,...e}},[t,e])}function n(e){let t;return t=e.disableParentContext?"function"==typeof e.components?e.components(r):e.components||r:i(e.components),a.createElement(s.Provider,{value:t},e.children)}}}]);
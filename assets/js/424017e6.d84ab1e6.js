"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["585"],{27121(e,t,o){o.r(t),o.d(t,{metadata:()=>r,default:()=>d,frontMatter:()=>i,contentTitle:()=>n,toc:()=>c,assets:()=>l});var r=JSON.parse('{"id":"user-guide/operator/authz","title":"Operator Authorization","description":"Policy surfaces for controller, backup, restore, and upgrade work, with destructive capabilities scoped to the right identities.","source":"@site/../docs/user-guide/operator/authz.md","sourceDirName":"user-guide/operator","slug":"/get-started/operator-authorization","permalink":"/openbao-operator/docs/next/get-started/operator-authorization","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/operator/authz.md","tags":[],"version":"current","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1776420928000,"frontMatter":{"title":"Operator Authorization","description":"Policy surfaces for controller, backup, restore, and upgrade work, with destructive capabilities scoped to the right identities.","slug":"/get-started/operator-authorization","hide_title":true,"pageType":"concept","journey":"get-started"},"sidebar":"operatorDocs","previous":{"title":"Operator authentication","permalink":"/openbao-operator/docs/next/get-started/operator-authentication"},"next":{"title":"Tenancy & Governance","permalink":"/openbao-operator/docs/next/tenant-onboarding"}}'),a=o(74848),s=o(28453);let i={title:"Operator Authorization",description:"Policy surfaces for controller, backup, restore, and upgrade work, with destructive capabilities scoped to the right identities.",slug:"/get-started/operator-authorization",hide_title:!0,pageType:"concept",journey:"get-started"},n,l={},c=[{value:"Default policy surfaces",id:"default-policy-surfaces",level:2},{value:"Official OpenBao background",id:"official-openbao-background",level:2}];function p(e){let t={a:"a",h2:"h2",li:"li",p:"p",ul:"ul",...(0,s.R)(),...e.components},{Callout:o,CommandBlock:r,DecisionTable:i,DiagramFrame:n,NextActions:l,PageHeader:c,TabItem:p,Tabs:d}=t;return o||u("Callout",!0),r||u("CommandBlock",!0),i||u("DecisionTable",!0),n||u("DiagramFrame",!0),l||u("NextActions",!0),c||u("PageHeader",!0),p||u("TabItem",!0),d||u("Tabs",!0),(0,a.jsxs)(a.Fragment,{children:[(0,a.jsx)(c,{title:"Operator authorization surfaces",lede:"Use separate policy surfaces for controller, backup, restore, and upgrade work. Each path authenticates separately and receives only the capabilities it needs."}),"\n",(0,a.jsx)(n,{title:"Policies stay attached to job-specific identities",caption:"Each operator path maps to its own JWT role and policy set, with separate controller, backup, restore, and upgrade identities.",code:`graph LR
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
  class PolicyRestore caution;`}),"\n",(0,a.jsx)(i,{title:"Policy surfaces by lifecycle capability",columns:["Policy surface","Used by","Typical capabilities","Why it stays separate"],rows:[{cells:["Controller maintenance","The main controller Deployment","`sys/health`, `sys/step-down`, and autopilot configuration reads or updates","Keep this path available for routine reconciliation and maintenance without adding destructive restore powers."],emphasis:"recommended"},{cells:["Backup","The generated backup Job","`sys/storage/raft/snapshot` read access","Snapshot reads are narrower than controller maintenance and remain easier to reason about independently."]},{cells:["Restore","The generated restore Job","`sys/storage/raft/snapshot-force` update access","Restore can replace the full cluster state and belongs only on the workload that performs restore."],emphasis:"caution"},{cells:["Upgrade","The generated upgrade Job","Step-down, autopilot state, snapshot read, and optional peer-management operations for blue-green flows","Upgrade paths often need time-bounded orchestration permissions without widening steady-state controller access."]}]}),"\n",(0,a.jsx)(o,{type:"danger",title:"Restore is a destructive role",children:(0,a.jsx)(t.p,{children:"The restore capability can replace data, policies, and keys across the cluster.\nDo not bind the restore policy to the controller or to a broad multi-purpose ServiceAccount just because it is convenient during setup."})}),"\n",(0,a.jsx)(t.h2,{id:"default-policy-surfaces",children:"Default policy surfaces"}),"\n",(0,a.jsxs)(d,{groupId:"operator-policy-surfaces",children:[(0,a.jsx)(p,{value:"controller",label:"Controller",children:(0,a.jsx)(r,{language:"hcl",label:"configure",title:"Controller maintenance policy",code:`path "sys/health" {
capabilities = ["read"]
}

path "sys/step-down" {
capabilities = ["sudo", "update"]
}

path "sys/storage/raft/autopilot/configuration" {
capabilities = ["read", "update"]
}`,children:(0,a.jsx)(t.p,{children:"This is the steady-state controller scope. Keep backup, restore, and blue-green peer management in separate roles unless you are intentionally changing the model."})})}),(0,a.jsx)(p,{value:"backup",label:"Backup",children:(0,a.jsx)(r,{language:"hcl",label:"configure",title:"Backup policy",code:`path "sys/storage/raft/snapshot" {
capabilities = ["read"]
}`,children:(0,a.jsx)(t.p,{children:"Backup only needs snapshot streaming. Storage-provider credentials are a separate boundary outside this OpenBao policy."})})}),(0,a.jsx)(p,{value:"restore",label:"Restore",children:(0,a.jsx)(r,{language:"hcl",label:"configure",title:"Restore policy",code:`path "sys/storage/raft/snapshot-force" {
capabilities = ["update"]
}`,children:(0,a.jsx)(t.p,{children:"Keep this policy tightly bound to the generated restore Job identity and only for the period where restore is actually needed."})})}),(0,a.jsx)(p,{value:"upgrade",label:"Upgrade",children:(0,a.jsx)(r,{language:"hcl",label:"configure",title:"Rolling and blue-green upgrade policy surfaces",code:`# Rolling upgrade baseline
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
}`,children:(0,a.jsx)(t.p,{children:"Rolling upgrades need less authority than blue-green cutovers. Keep those strategies separate in your head when you review the required policy surface."})})})]}),"\n",(0,a.jsx)(i,{kind:"reference",title:"Common authorization failures",columns:["Symptom","Likely boundary","Check first"],rows:[{cells:["JWT login succeeds but the request returns `permission denied`","The workload authenticated correctly but the policy is missing the needed path capability","Which job or controller path is making the request, then the matching policy surface"],emphasis:"recommended"},{cells:["Backup works but restore fails","The restore Job identity or restore policy is missing or misbound","Restore ServiceAccount, restore role binding, and `snapshot-force` policy"]},{cells:["Rolling upgrade works but blue-green cutover stalls","Peer-management permissions were not added for the upgrade strategy in use","Upgrade strategy and the corresponding upgrade policy paths"]},{cells:["Controller can do too much","A shortcut merged job-specific capabilities into the controller role","Manual auth configuration drift from the intended separation model"]}]}),"\n",(0,a.jsx)(l,{title:"Go deeper",items:[{label:"Operator authentication",description:"Return to the JWT audience and bound-subject model when auth fails before policy even matters.",docId:"user-guide/operator/authn"},{label:"Backup operations",description:"See how the backup Job uses its own auth and storage credentials during normal operation.",docId:"user-guide/openbaocluster/operations/backups"},{label:"Restore manager architecture",description:"Review why restore stays isolated from the controller and how the operator drives it.",docId:"architecture/restore-manager"}]}),"\n",(0,a.jsx)(t.h2,{id:"official-openbao-background",children:"Official OpenBao background"}),"\n",(0,a.jsxs)(t.ul,{children:["\n",(0,a.jsx)(t.li,{children:(0,a.jsx)(t.a,{href:"https://openbao.org/docs/concepts/policies/",children:"Policy concepts"})}),"\n",(0,a.jsx)(t.li,{children:(0,a.jsx)(t.a,{href:"https://openbao.org/docs/commands/policy/",children:"Policy command reference"})}),"\n",(0,a.jsx)(t.li,{children:(0,a.jsx)(t.a,{href:"https://openbao.org/docs/concepts/tokens/",children:"Token concepts"})}),"\n"]})]})}function d(e={}){let{wrapper:t}={...(0,s.R)(),...e.components};return t?(0,a.jsx)(t,{...e,children:(0,a.jsx)(p,{...e})}):p(e)}function u(e,t){throw Error("Expected "+(t?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},28453(e,t,o){o.d(t,{R:()=>i,x:()=>n});var r=o(96540);let a={},s=r.createContext(a);function i(e){let t=r.useContext(s);return r.useMemo(function(){return"function"==typeof e?e(t):{...t,...e}},[t,e])}function n(e){let t;return t=e.disableParentContext?"function"==typeof e.components?e.components(a):e.components||a:i(e.components),r.createElement(s.Provider,{value:t},e.children)}}}]);
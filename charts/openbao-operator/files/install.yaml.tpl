apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-controller
  namespace: {{ .Release.Namespace }}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-provisioner
  namespace: {{ .Release.Namespace }}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-provisioner-delegate
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  labels:
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-leader-election-role
  namespace: {{ .Release.Namespace }}
rules:
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - patch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: openbao-operator-metrics-auth-role
rules:
- apiGroups:
  - authentication.k8s.io
  resources:
  - tokenreviews
  verbs:
  - create
- apiGroups:
  - authorization.k8s.io
  resources:
  - subjectaccessreviews
  verbs:
  - create
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: openbao-operator-metrics-reader
rules:
- nonResourceURLs:
  - /metrics
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-controller-openbaocluster-role
rules:
- apiGroups:
  - openbao.org
  resources:
  - openbaoclusters
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - validatingadmissionpolicies
  - validatingadmissionpolicybindings
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-controller-openbaorestore-role
rules:
- apiGroups:
  - openbao.org
  resources:
  - openbaorestores
  verbs:
  - get
  - list
  - watch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-provisioner-role
rules:
- apiGroups:
  - ""
  resources:
  - namespaces
  verbs:
  - get
  - update
  - patch
- apiGroups:
  - ""
  resourceNames:
  - openbao-operator-provisioner-delegate
  resources:
  - serviceaccounts
  verbs:
  - impersonate
- apiGroups:
  - ""
  resources:
  - serviceaccounts
  verbs:
  - get
- apiGroups:
  - openbao.org
  resources:
  - openbaotenants
  - openbaotenants/status
  - openbaotenants/finalizers
  verbs:
  - get
  - list
  - watch
  - update
  - patch
- apiGroups:
  - openbao.org
  resources:
  - openbaoclusters
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - roles
  - rolebindings
  verbs:
  - get
  - list
  - watch
  - delete
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-tenant-template
rules:
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - roles
  - rolebindings
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - openbao.org
  resources:
  - openbaoclusters
  - openbaoclusters/status
  - openbaoclusters/finalizers
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - openbao.org
  resources:
  - openbaorestores
  - openbaorestores/status
  - openbaorestores/finalizers
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - statefulsets
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - deployments
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - services
  - configmaps
  - serviceaccounts
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - patch
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - get
  - list
  - watch
  - update
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - persistentvolumeclaims
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - policy
  resources:
  - poddisruptionbudgets
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - networking.k8s.io
  resources:
  - ingresses
  - networkpolicies
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - httproutes
  - tlsroutes
  - backendtlspolicies
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - endpoints
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - discovery.k8s.io
  resources:
  - endpointslices
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - get
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - create
  - delete
  - get
  - patch
  - update
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbaocluster-admin-role
rules:
- apiGroups:
  - openbao.org
  resources:
  - openbaoclusters
  verbs:
  - '*'
- apiGroups:
  - openbao.org
  resources:
  - openbaoclusters/status
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbaocluster-editor-role
rules:
- apiGroups:
  - openbao.org
  resources:
  - openbaoclusters
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - openbao.org
  resources:
  - openbaoclusters/status
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbaocluster-viewer-role
rules:
- apiGroups:
  - openbao.org
  resources:
  - openbaoclusters
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - openbao.org
  resources:
  - openbaoclusters/status
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    rbac.authorization.k8s.io/aggregate-to-admin: "true"
    rbac.authorization.k8s.io/aggregate-to-edit: "true"
  name: openbao-operator-openbaotenant-editor-role
rules:
- apiGroups:
  - openbao.org
  resources:
  - openbaotenants
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - openbao.org
  resources:
  - openbaotenants/status
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-controller-leader-election-rolebinding
  namespace: {{ .Release.Namespace }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: openbao-operator-leader-election-role
subjects:
- kind: ServiceAccount
  name: openbao-operator-controller
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  labels:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-provisioner-leader-election-rolebinding
  namespace: {{ .Release.Namespace }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: openbao-operator-leader-election-role
subjects:
- kind: ServiceAccount
  name: openbao-operator-openbao-operator-provisioner
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-controller-metrics-auth-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: openbao-operator-metrics-auth-role
subjects:
- kind: ServiceAccount
  name: openbao-operator-controller
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-controller-openbaocluster-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: openbao-operator-openbao-operator-controller-openbaocluster-role
subjects:
- kind: ServiceAccount
  name: openbao-operator-controller
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-controller-openbaorestore-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: openbao-operator-openbao-operator-controller-openbaorestore-role
subjects:
- kind: ServiceAccount
  name: openbao-operator-controller
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-provisioner-metrics-auth-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: openbao-operator-metrics-auth-role
subjects:
- kind: ServiceAccount
  name: openbao-operator-openbao-operator-provisioner
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-provisioner-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: openbao-operator-openbao-operator-provisioner-role
subjects:
- kind: ServiceAccount
  name: openbao-operator-openbao-operator-provisioner
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-tenant-delegate-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: openbao-operator-openbao-operator-tenant-template
subjects:
- kind: ServiceAccount
  name: openbao-operator-provisioner-delegate
  namespace: {{ .Release.Namespace }}
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-controller-metrics-service
  namespace: {{ .Release.Namespace }}
spec:
  ports:
  - name: https
    port: 8443
    protocol: TCP
    targetPort: 8443
  selector:
    app.kubernetes.io/component: controller
    app.kubernetes.io/name: openbao-operator
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-provisioner-metrics-service
  namespace: {{ .Release.Namespace }}
spec:
  ports:
  - name: https
    port: 8443
    protocol: TCP
    targetPort: 8443
  selector:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/name: openbao-operator
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/component: controller
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-controller
  namespace: {{ .Release.Namespace }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/component: controller
      app.kubernetes.io/name: openbao-operator
  template:
    metadata:
      annotations:
        kubectl.kubernetes.io/default-container: manager
      labels:
        app.kubernetes.io/component: controller
        app.kubernetes.io/name: openbao-operator
    spec:
      containers:
      - args:
        - --leader-elect
        - --health-probe-bind-address=:8081
        - --metrics-bind-address=:8443
        - --metrics-bind-address=:8443
        command:
        - /manager
        - controller
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: OPERATOR_SERVICE_ACCOUNT_NAME
          value: openbao-operator-controller
        - name: OPERATOR_VERSION
          value: {{ include "openbao-operator.operatorVersion" . | quote }}
        - name: OPERATOR_SENTINEL_IMAGE_REPOSITORY
          value: {{ include "openbao-operator.sentinelImageRepository" . | quote }}
        image: {{ include "openbao-operator.managerImage" . }}
        imagePullPolicy: {{ .Values.image.pullPolicy }}
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        name: manager
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          limits:
            cpu: 500m
            memory: 256Mi
          requests:
            cpu: 50m
            memory: 128Mi
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
          readOnlyRootFilesystem: true
          runAsGroup: 65532
          runAsUser: 65532
        volumeMounts:
        - mountPath: /var/run/secrets/tokens
          name: openbao-token
          readOnly: true
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      serviceAccountName: openbao-operator-controller
      terminationGracePeriodSeconds: 10
      volumes:
      - name: openbao-token
        projected:
          sources:
          - serviceAccountToken:
              audience: openbao-internal
              expirationSeconds: 3600
              path: openbao-token
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/component: provisioner
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: openbao-operator
  name: openbao-operator-openbao-operator-provisioner
  namespace: {{ .Release.Namespace }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/component: provisioner
      app.kubernetes.io/name: openbao-operator
  template:
    metadata:
      annotations:
        kubectl.kubernetes.io/default-container: manager
      labels:
        app.kubernetes.io/component: provisioner
        app.kubernetes.io/name: openbao-operator
    spec:
      containers:
      - args:
        - --leader-elect
        - --health-probe-bind-address=:8081
        - --metrics-bind-address=:8443
        - --metrics-bind-address=:8443
        command:
        - /manager
        - provisioner
        env:
        - name: OPERATOR_VERSION
          value: {{ include "openbao-operator.operatorVersion" . | quote }}
        - name: OPERATOR_SENTINEL_IMAGE_REPOSITORY
          value: {{ include "openbao-operator.sentinelImageRepository" . | quote }}
        image: {{ include "openbao-operator.managerImage" . }}
        imagePullPolicy: {{ .Values.image.pullPolicy }}
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        name: manager
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          limits:
            cpu: 100m
            memory: 64Mi
          requests:
            cpu: 10m
            memory: 32Mi
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
          readOnlyRootFilesystem: true
          runAsGroup: 65532
          runAsUser: 65532
        volumeMounts: []
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      serviceAccountName: openbao-operator-openbao-operator-provisioner
      terminationGracePeriodSeconds: 10
      volumes: []
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: openbao-operator-lock-controller-statefulset-mutations
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
    - apiGroups:
      - apps
      apiVersions:
      - v1
      operations:
      - UPDATE
      resources:
      - statefulsets
  validations:
  - expression: '!variables.is_controller || object.spec.template.spec.volumes ==
      oldObject.spec.template.spec.volumes'
    message: OpenBao controller cannot modify StatefulSet volumes.
  - expression: '!variables.is_controller || object.spec.template.spec.containers.map(c,
      {''name'': c.name, ''command'': c.command, ''args'': c.args}) == oldObject.spec.template.spec.containers.map(c,
      {''name'': c.name, ''command'': c.command, ''args'': c.args})'
    message: OpenBao controller cannot modify container commands or args.
  - expression: |-
      !variables.is_controller || (has(object.spec.template.spec.initContainers) ? object.spec.template.spec.initContainers : [])

        .map(c, {'name': c.name, 'command': c.command, 'args': c.args}) ==
      (has(oldObject.spec.template.spec.initContainers) ? oldObject.spec.template.spec.initContainers : [])

        .map(c, {'name': c.name, 'command': c.command, 'args': c.args})
    message: OpenBao controller cannot modify init container commands or args.
  - expression: |-
      !variables.is_controller || object.spec.template.spec.automountServiceAccountToken ==

        oldObject.spec.template.spec.automountServiceAccountToken
    message: OpenBao controller cannot modify automountServiceAccountToken.
  - expression: '!variables.is_controller || object.spec.template.spec.securityContext
      == oldObject.spec.template.spec.securityContext'
    message: OpenBao controller cannot modify pod securityContext.
  - expression: '!variables.is_controller || object.spec.template.spec.containers.map(c,
      {''name'': c.name, ''securityContext'': c.securityContext, ''volumeMounts'':
      c.volumeMounts}) == oldObject.spec.template.spec.containers.map(c, {''name'':
      c.name, ''securityContext'': c.securityContext, ''volumeMounts'': c.volumeMounts})'
    message: OpenBao controller cannot modify container securityContext or volumeMounts.
  variables:
  - expression: request.userInfo.username == "system:serviceaccount:{{ .Release.Namespace }}:openbao-operator-controller"
    name: is_controller
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: openbao-operator-lock-managed-resource-mutations
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
    - apiGroups:
      - ""
      apiVersions:
      - v1
      operations:
      - CREATE
      - UPDATE
      - DELETE
      resources:
      - secrets
      - configmaps
      - services
      - pods
      - endpoints
    - apiGroups:
      - discovery.k8s.io
      apiVersions:
      - v1
      operations:
      - CREATE
      - UPDATE
      - DELETE
      resources:
      - endpointslices
    - apiGroups:
      - apps
      apiVersions:
      - v1
      operations:
      - CREATE
      - UPDATE
      - DELETE
      resources:
      - statefulsets
    - apiGroups:
      - batch
      apiVersions:
      - v1
      operations:
      - CREATE
      - UPDATE
      - DELETE
      resources:
      - jobs
    - apiGroups:
      - networking.k8s.io
      apiVersions:
      - v1
      operations:
      - CREATE
      - UPDATE
      - DELETE
      resources:
      - ingresses
      - networkpolicies
    - apiGroups:
      - gateway.networking.k8s.io
      apiVersions:
      - v1
      operations:
      - CREATE
      - UPDATE
      - DELETE
      resources:
      - httproutes
      - backendtlspolicies
    - apiGroups:
      - gateway.networking.k8s.io
      apiVersions:
      - v1alpha2
      operations:
      - CREATE
      - UPDATE
      - DELETE
      resources:
      - tlsroutes
  validations:
  - expression: '!variables.is_managed || variables.is_operator_controller || (variables.is_kube_controller_manager
      && (request.operation == "DELETE" || request.resource.resource == "pods")) ||
      (variables.is_system_controller && request.operation == "DELETE") ||
      (variables.is_system_controller && request.resource.resource == "pods") ||
      (variables.is_system_controller && variables.is_endpoints_controller_request) ||
      (variables.is_kube_system_controller && request.resource.resource == "pods" &&
      (request.operation == "CREATE" || request.operation == "DELETE")) ||
      (variables.is_kube_system_controller && variables.is_endpoints_controller_request)
      || variables.is_cert_manager || (variables.is_pod_request && variables.is_kubelet_node
      && request.operation == "DELETE") || variables.is_service_registration_label_update
      || (variables.maintenance_enabled && variables.is_break_glass_admin)'
    message: Direct modification of OpenBao-managed resources is prohibited; modify
      the parent OpenBaoCluster/OpenBaoTenant instead.
  variables:
  - expression: |-
      request.operation == "DELETE" ?

        (oldObject != null &&
          has(oldObject.metadata) &&
          has(oldObject.metadata.labels) &&
          ("app.kubernetes.io/managed-by" in oldObject.metadata.labels) &&
          oldObject.metadata.labels["app.kubernetes.io/managed-by"] == "openbao-operator") :
        ((object != null &&
            has(object.metadata) &&
            has(object.metadata.labels) &&
            ("app.kubernetes.io/managed-by" in object.metadata.labels) &&
            object.metadata.labels["app.kubernetes.io/managed-by"] == "openbao-operator") ||
          (oldObject != null &&
            has(oldObject.metadata) &&
            has(oldObject.metadata.labels) &&
            ("app.kubernetes.io/managed-by" in oldObject.metadata.labels) &&
            oldObject.metadata.labels["app.kubernetes.io/managed-by"] == "openbao-operator"))
    name: is_managed
  - expression: request.userInfo.username == "system:serviceaccount:{{ .Release.Namespace }}:openbao-operator-controller"
    name: is_operator_controller
  - expression: request.userInfo.username == "system:kube-controller-manager" || request.userInfo.username
      == "system:serviceaccount:kube-system:kube-controller-manager"
    name: is_kube_controller_manager
  - expression: request.userInfo.username.startsWith("system:controller:")
    name: is_system_controller
  - expression: request.userInfo.groups.exists(g, g == "system:serviceaccounts:kube-system")
      || request.userInfo.username.startsWith("system:serviceaccount:kube-system:")
    name: is_kube_system_controller
  - expression: request.resource.resource in ["endpoints", "endpointslices"]
    name: is_endpoints_controller_request
  - expression: request.userInfo.username.startsWith("system:serviceaccount:cert-manager:")
    name: is_cert_manager
  - expression: request.kind.group == "" && request.kind.kind == "Pod"
    name: is_pod_request
  - expression: request.userInfo.username.startsWith("system:node:")
    name: is_kubelet_node
  - expression: |-
      object != null && has(object.metadata) && has(object.metadata.labels) && ("openbao.org/cluster" in object.metadata.labels) ?

        object.metadata.labels["openbao.org/cluster"] :
        (object != null && has(object.metadata) && has(object.metadata.labels) && ("app.kubernetes.io/instance" in object.metadata.labels) ?
          object.metadata.labels["app.kubernetes.io/instance"] : "")
    name: pod_cluster_name
  - expression: |-
      variables.pod_cluster_name != "" && request.userInfo.username ==

        ("system:serviceaccount:" + request.namespace + ":" + variables.pod_cluster_name + "-serviceaccount")
    name: is_cluster_serviceaccount
  - expression: |-
      variables.is_pod_request && request.operation == "UPDATE" && variables.is_cluster_serviceaccount && object.spec == oldObject.spec && (has(object.metadata.annotations) == has(oldObject.metadata.annotations) &&

        (!has(object.metadata.annotations) || object.metadata.annotations == oldObject.metadata.annotations)) &&
      (has(object.metadata.ownerReferences) == has(oldObject.metadata.ownerReferences) &&

        (!has(object.metadata.ownerReferences) || object.metadata.ownerReferences == oldObject.metadata.ownerReferences)) &&
      (has(object.metadata.finalizers) == has(oldObject.metadata.finalizers) &&

        (!has(object.metadata.finalizers) || object.metadata.finalizers == oldObject.metadata.finalizers)) &&
      has(object.metadata.labels) && has(oldObject.metadata.labels) && (

        (
          object.metadata.labels.all(k, v,
            k in [
              "openbao-active",
              "openbao-initialized",
              "openbao-sealed",
              "openbao-perf-standby",
              "openbao-version"
            ] || (k in oldObject.metadata.labels && oldObject.metadata.labels[k] == v)) &&
          oldObject.metadata.labels.all(k, v,
            k in [
              "openbao-active",
              "openbao-initialized",
              "openbao-sealed",
              "openbao-perf-standby",
              "openbao-version"
            ] || (k in object.metadata.labels && object.metadata.labels[k] == v))
        )
      )
    name: is_service_registration_label_update
  - expression: |-
      request.operation == "DELETE" ?

        (oldObject != null &&
          has(oldObject.metadata) &&
          has(oldObject.metadata.annotations) &&
          ("openbao.org/maintenance" in oldObject.metadata.annotations) &&
          oldObject.metadata.annotations["openbao.org/maintenance"] == "true") :
        ((object != null &&
            has(object.metadata) &&
            has(object.metadata.annotations) &&
            ("openbao.org/maintenance" in object.metadata.annotations) &&
            object.metadata.annotations["openbao.org/maintenance"] == "true") ||
          (oldObject != null &&
            has(oldObject.metadata) &&
            has(oldObject.metadata.annotations) &&
            ("openbao.org/maintenance" in oldObject.metadata.annotations) &&
            oldObject.metadata.annotations["openbao.org/maintenance"] == "true"))
    name: maintenance_enabled
  - expression: request.userInfo.groups.exists(g, g == "system:masters")
    name: is_break_glass_admin
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: openbao-operator-openbao-restrict-provisioner-delegate
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
    - apiGroups:
      - rbac.authorization.k8s.io
      apiVersions:
      - v1
      operations:
      - CREATE
      - UPDATE
      resources:
      - roles
      - rolebindings
  validations:
  - expression: |-
      !variables.is_delegate || request.resource.resource != 'roles' || object.metadata.name in [

        'openbao-operator-tenant-role',
        'openbao-sentinel-role',
        'openbao-operator-tenant-secrets-reader',
        'openbao-operator-tenant-secrets-writer'
      ]
    message: The Provisioner Delegate can only create Roles for the operator tenant
      template (tenant/sentinel/secrets allowlists).
  - expression: |-
      !variables.is_delegate || request.resource.resource != 'rolebindings' || object.metadata.name in [

        'openbao-operator-tenant-rolebinding',
        'openbao-sentinel-rolebinding',
        'openbao-operator-tenant-secrets-reader-rolebinding',
        'openbao-operator-tenant-secrets-writer-rolebinding'
      ]
    message: The Provisioner Delegate can only create RoleBindings for the operator
      tenant template (tenant/sentinel/secrets allowlists).
  - expression: |-
      !variables.is_delegate || request.resource.resource != 'rolebindings' || object.roleRef.name in [

        'openbao-operator-tenant-role',
        'openbao-sentinel-role',
        'openbao-operator-tenant-secrets-reader',
        'openbao-operator-tenant-secrets-writer'
      ]
    message: The Provisioner Delegate is restricted to binding only the operator tenant
      template Roles (tenant/sentinel/secrets allowlists).
  - expression: '!variables.is_delegate || request.resource.resource != ''rolebindings''
      || object.subjects.all(s, s.kind == ''ServiceAccount'')'
    message: The Provisioner Delegate can only grant permissions to ServiceAccounts.
  - expression: '!variables.is_delegate || request.resource.resource != ''rolebindings''
      || object.subjects.all(s, !has(s.apiGroup) || s.apiGroup == '''')'
    message: The Provisioner Delegate can only bind to core ServiceAccount subjects
      (apiGroup must be empty).
  - expression: '!variables.is_delegate || request.resource.resource != ''rolebindings''
      || (object.roleRef.kind == ''Role'' && object.roleRef.apiGroup == ''rbac.authorization.k8s.io'')'
    message: The Provisioner Delegate can only create RoleBindings that reference
      a Role (not ClusterRole).
  - expression: |-
      !variables.is_delegate || request.resource.resource != 'rolebindings' || (

        (
          object.metadata.name == 'openbao-operator-tenant-rolebinding' &&
          object.roleRef.name == 'openbao-operator-tenant-role' &&
          object.subjects.size() == 1 &&
          object.subjects[0].kind == 'ServiceAccount' &&
          object.subjects[0].name == 'openbao-operator-controller' &&
          object.subjects[0].namespace == 'openbao-operator-system'
        ) ||
        (
          object.metadata.name == 'openbao-sentinel-rolebinding' &&
          object.roleRef.name == 'openbao-sentinel-role' &&
          object.subjects.size() == 1 &&
          object.subjects[0].kind == 'ServiceAccount' &&
          object.subjects[0].name == 'openbao-sentinel' &&
          object.subjects[0].namespace == object.metadata.namespace
        ) ||
        (
          object.metadata.name == 'openbao-operator-tenant-secrets-reader-rolebinding' &&
          object.roleRef.name == 'openbao-operator-tenant-secrets-reader' &&
          object.subjects.size() == 1 &&
          object.subjects[0].kind == 'ServiceAccount' &&
          object.subjects[0].name == 'openbao-operator-controller' &&
          object.subjects[0].namespace == 'openbao-operator-system'
        ) ||
        (
          object.metadata.name == 'openbao-operator-tenant-secrets-writer-rolebinding' &&
          object.roleRef.name == 'openbao-operator-tenant-secrets-writer' &&
          object.subjects.size() == 1 &&
          object.subjects[0].kind == 'ServiceAccount' &&
          object.subjects[0].name == 'openbao-operator-controller' &&
          object.subjects[0].namespace == 'openbao-operator-system'
        )
      )
    message: The Provisioner Delegate can only bind tenant RBAC to the operator controller
      ServiceAccount, or sentinel RBAC to the tenant-local openbao-sentinel ServiceAccount.
  - expression: |-
      !variables.is_delegate || request.resource.resource != 'roles' || !has(object.rules) || object.rules == null || !object.rules.exists(rule,

        has(rule.verbs) &&
        rule.verbs != null &&
        rule.verbs.exists(v,
          v == 'impersonate' ||
          v == 'bind' ||
          v == 'escalate' ||
          v == '*'
        )
      )
    message: The Provisioner Delegate cannot create Roles granting 'impersonate',
      'bind', 'escalate', or wildcard permissions.
  - expression: |-
      !variables.is_delegate || request.resource.resource != 'roles' || !has(object.rules) || object.rules == null || !object.rules.exists(rule,

        (
          has(rule.apiGroups) &&
          rule.apiGroups != null &&
          rule.apiGroups.exists(g, g == '*')
        ) ||
        (
          has(rule.resources) &&
          rule.resources != null &&
          rule.resources.exists(r, r == '*')
        )
      )
    message: The Provisioner Delegate cannot create Roles with wildcard apiGroups
      or resources.
  - expression: |-
      !variables.is_delegate || request.resource.resource != 'roles' || !has(object.rules) || object.rules == null || !object.rules.exists(rule,

        has(rule.nonResourceURLs) &&
        rule.nonResourceURLs != null &&
        rule.nonResourceURLs.size() > 0
      )
    message: The Provisioner Delegate cannot create Roles granting nonResourceURLs.
  - expression: |-
      !variables.is_delegate || request.resource.resource != 'roles' || !has(object.rules) || object.rules == null || !object.rules.exists(rule,

        has(rule.resources) &&
        rule.resources != null &&
        rule.resources.exists(r, r == 'secrets') &&
        (
          object.metadata.name == 'openbao-sentinel-role' ||
          (
            has(rule.verbs) &&
            rule.verbs != null &&
            rule.verbs.exists(v, v == 'list' || v == 'watch' || v == '*')
          )
        )
      )
    message: The Provisioner Delegate cannot grant Secrets list/watch, and Sentinel
      RBAC must not include Secrets permissions.
  variables:
  - expression: request.userInfo.username == "system:serviceaccount:{{ .Release.Namespace }}:openbao-operator-provisioner-delegate"
    name: is_delegate
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: openbao-operator-openbao-restrict-sentinel-mutations
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
    - apiGroups:
      - openbao.org
      apiVersions:
      - v1alpha1
      operations:
      - UPDATE
      resources:
      - openbaoclusters/status
  validations:
  - expression: |-
      !variables.is_sentinel || (
        (has(variables.new_status.phase) ? variables.new_status.phase : "") == (has(variables.old_status.phase) ? variables.old_status.phase : "") &&
        (has(variables.new_status.activeLeader) ? variables.new_status.activeLeader : "") == (has(variables.old_status.activeLeader) ? variables.old_status.activeLeader : "") &&
        (has(variables.new_status.readyReplicas) ? variables.new_status.readyReplicas : 0) == (has(variables.old_status.readyReplicas) ? variables.old_status.readyReplicas : 0) &&
        (has(variables.new_status.currentVersion) ? variables.new_status.currentVersion : "") == (has(variables.old_status.currentVersion) ? variables.old_status.currentVersion : "") &&
        (has(variables.new_status.initialized) ? variables.new_status.initialized : false) == (has(variables.old_status.initialized) ? variables.old_status.initialized : false) &&
        (has(variables.new_status.selfInitialized) ? variables.new_status.selfInitialized : false) == (has(variables.old_status.selfInitialized) ? variables.old_status.selfInitialized : false) &&
        (has(variables.new_status.lastBackupTime) ? variables.new_status.lastBackupTime : null) == (has(variables.old_status.lastBackupTime) ? variables.old_status.lastBackupTime : null) &&
        (has(variables.new_status.upgrade) ? variables.new_status.upgrade : null) == (has(variables.old_status.upgrade) ? variables.old_status.upgrade : null) &&
        (has(variables.new_status.backup) ? variables.new_status.backup : null) == (has(variables.old_status.backup) ? variables.old_status.backup : null) &&
        (has(variables.new_status.drift) ? variables.new_status.drift : null) == (has(variables.old_status.drift) ? variables.old_status.drift : null) &&
        (has(variables.new_status.blueGreen) ? variables.new_status.blueGreen : null) == (has(variables.old_status.blueGreen) ? variables.old_status.blueGreen : null) &&
        (has(variables.new_status.operationLock) ? variables.new_status.operationLock : null) == (has(variables.old_status.operationLock) ? variables.old_status.operationLock : null) &&
        (has(variables.new_status.breakGlass) ? variables.new_status.breakGlass : null) == (has(variables.old_status.breakGlass) ? variables.old_status.breakGlass : null) &&
        (has(variables.new_status.workload) ? variables.new_status.workload : null) == (has(variables.old_status.workload) ? variables.old_status.workload : null) &&
        (has(variables.new_status.adminOps) ? variables.new_status.adminOps : null) == (has(variables.old_status.adminOps) ? variables.old_status.adminOps : null) &&
        (has(variables.new_status.conditions) ? variables.new_status.conditions : []) == (has(variables.old_status.conditions) ? variables.old_status.conditions : []) &&

        has(variables.new_status.sentinel) &&
        (
          (!has(variables.old_status.sentinel) &&
            !has(variables.new_status.sentinel.lastHandledTriggerID) &&
            !has(variables.new_status.sentinel.lastHandledAt)
          ) ||
          (has(variables.old_status.sentinel) &&
            ((!has(variables.new_status.sentinel.lastHandledTriggerID) && !has(variables.old_status.sentinel.lastHandledTriggerID)) ||
              (has(variables.new_status.sentinel.lastHandledTriggerID) && has(variables.old_status.sentinel.lastHandledTriggerID) &&
                variables.new_status.sentinel.lastHandledTriggerID == variables.old_status.sentinel.lastHandledTriggerID)) &&
            ((!has(variables.new_status.sentinel.lastHandledAt) && !has(variables.old_status.sentinel.lastHandledAt)) ||
              (has(variables.new_status.sentinel.lastHandledAt) && has(variables.old_status.sentinel.lastHandledAt) &&
                variables.new_status.sentinel.lastHandledAt == variables.old_status.sentinel.lastHandledAt))
          )
        )
      )
    message: The Sentinel may only update status.sentinel trigger fields (triggerID/triggeredAt/triggerResource).
      Spec, metadata, and all other status fields are forbidden.
  variables:
  - expression: request.userInfo.username.startsWith("system:serviceaccount:") &&
      request.userInfo.username.endsWith(":openbao-sentinel")
    name: is_sentinel
  - expression: 'has(object.status) ? object.status : object'
    name: new_status
  - expression: 'has(oldObject.status) ? oldObject.status : oldObject'
    name: old_status
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: openbao-operator-validate-openbaocluster
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
    - apiGroups:
      - openbao.org
      apiVersions:
      - v1alpha1
      operations:
      - CREATE
      - UPDATE
      resources:
      - openbaoclusters
  validations:
  - expression: |-
      !has(object.spec.profile) || object.spec.profile != "Hardened" || (object.spec.tls.mode in ["External", "ACME"] &&

        has(object.spec.unseal) &&
        object.spec.unseal.type != "static" &&
        has(object.spec.selfInit) &&
        object.spec.selfInit.enabled == true &&
        !(has(object.spec.unseal.transit) && has(object.spec.unseal.transit.tlsSkipVerify) && object.spec.unseal.transit.tlsSkipVerify == true) &&
        !(has(object.spec.unseal.kmip) && has(object.spec.unseal.kmip.tlsSkipVerify) && object.spec.unseal.kmip.tlsSkipVerify == true))
    message: Hardened profile requires TLS mode External or ACME, an external unseal
      (non-static), self-init enabled, and disallows tlsSkipVerify=true in seal configuration.
  - expression: has(object.spec.initContainer) && object.spec.initContainer.enabled
      == true && object.spec.initContainer.image != ""
    message: spec.initContainer is required, must be enabled, and must specify a non-empty
      image.
  - expression: '!has(object.spec.selfInit) || object.spec.selfInit.enabled == false
      || (has(object.spec.selfInit.bootstrapJWTAuth) && object.spec.selfInit.bootstrapJWTAuth
      == true) || (has(object.spec.selfInit.requests) && size(object.spec.selfInit.requests)
      > 0)'
    message: spec.selfInit.requests is required and must not be empty when spec.selfInit.enabled
      is true unless spec.selfInit.bootstrapJWTAuth is true.
  - expression: |-
      !has(object.spec.selfInit) || object.spec.selfInit.enabled == false || (!has(object.spec.selfInit.requests) ||

        (size(object.spec.selfInit.requests) <= 64 &&
          object.spec.selfInit.requests.all(r,
            object.spec.selfInit.requests.filter(x, x.name == r.name).size() == 1) &&
          object.spec.selfInit.requests.all(r, size(r.path) <= 256)))
    message: spec.selfInit.requests must have unique names (max 64) and request paths
      must be <= 256 characters.
  - expression: '!has(object.spec.gateway) || object.spec.gateway.enabled == false
      || (object.spec.gateway.hostname != "" && object.spec.gateway.gatewayRef.name
      != "")'
    message: spec.gateway.hostname and spec.gateway.gatewayRef.name are required when
      Gateway is enabled.
  - expression: |-
      !variables.has_backup || (object.spec.backup.schedule != "" &&

        object.spec.backup.executorImage != "" &&
        (object.spec.backup.jwtAuthRole != "" || has(object.spec.backup.tokenSecretRef)) &&
        (object.spec.backup.target.roleArn != "" || has(object.spec.backup.target.credentialsSecretRef)) &&
        object.spec.backup.schedule.matches("^\\S+\\s+\\S+\\s+\\S+\\s+\\S+\\s+\\S+$"))
    message: spec.backup requires schedule, executorImage, OpenBao auth (jwtAuthRole
      or tokenSecretRef), storage auth (target.roleArn or target.credentialsSecretRef),
      and a 5-field cron expression.
  - expression: |-
      !has(object.spec.profile) || object.spec.profile != "Hardened" || !(variables.has_backup || variables.has_pre_upgrade_snapshot || variables.has_bluegreen_pre_upgrade_snapshot) || (has(object.spec.network) &&

        has(object.spec.network.egressRules) &&
        size(object.spec.network.egressRules) > 0)
    message: Hardened profile requires non-empty spec.network.egressRules when backups
      or pre-upgrade snapshots are enabled.
  - expression: |-
      !variables.has_backup || !has(object.spec.backup.target.endpoint) || object.spec.backup.target.endpoint == "" || !(

        object.spec.backup.target.endpoint.contains("localhost") ||
        object.spec.backup.target.endpoint.contains("127.0.0.1") ||
        object.spec.backup.target.endpoint.contains("::1")
      )
    message: Backup endpoint cannot point to localhost (SSRF protection).
  - expression: '!variables.has_backup || !has(object.spec.backup.target.endpoint)
      || object.spec.backup.target.endpoint == "" || !object.spec.backup.target.endpoint.matches("^https?://169\\.254\\..*")'
    message: Backup endpoint cannot point to link-local addresses (SSRF protection
      against cloud metadata services).
  - expression: '!has(object.spec.profile) || object.spec.profile != "Hardened" ||
      !variables.has_backup || !has(object.spec.backup.target.endpoint) || object.spec.backup.target.endpoint
      == "" || object.spec.backup.target.endpoint.startsWith("https://") || object.spec.backup.target.endpoint.startsWith("s3://")'
    message: Backup endpoint must use HTTPS or S3 scheme in Hardened profile.
  variables:
  - expression: has(object.spec.backup)
    name: has_backup
  - expression: has(object.spec.upgrade) && object.spec.upgrade.preUpgradeSnapshot
      == true
    name: has_pre_upgrade_snapshot
  - expression: has(object.spec.upgrade) && has(object.spec.upgrade.blueGreen) &&
      object.spec.upgrade.blueGreen.preUpgradeSnapshot == true
    name: has_bluegreen_pre_upgrade_snapshot
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: openbao-operator-lock-controller-statefulset-mutations
spec:
  policyName: openbao-operator-lock-controller-statefulset-mutations
  validationActions:
  - Deny
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: openbao-operator-lock-managed-resource-mutations
spec:
  policyName: openbao-operator-lock-managed-resource-mutations
  validationActions:
  - Deny
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: openbao-operator-openbao-restrict-provisioner-delegate-binding
spec:
  matchResources:
    resourceRules:
    - apiGroups:
      - rbac.authorization.k8s.io
      apiVersions:
      - v1
      operations:
      - CREATE
      - UPDATE
      resources:
      - roles
      - rolebindings
  policyName: openbao-operator-openbao-restrict-provisioner-delegate
  validationActions:
  - Deny
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: openbao-operator-openbao-restrict-sentinel-mutations
spec:
  policyName: openbao-operator-openbao-restrict-sentinel-mutations
  validationActions:
  - Deny
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: openbao-operator-validate-openbaocluster
spec:
  policyName: openbao-operator-validate-openbaocluster
  validationActions:
  - Deny

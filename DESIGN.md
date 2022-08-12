# Secrets Operator

Goals:
* Synchronize secrets from one namespace to another
* Derive secrets from other secrets using templating
* Do so securely

Resource Kinds:

apiVersion: secrets.meln5674.github.com/v1alpha1
kind: DerivedSecret
metadata:
  name: my-derived-secret
  namespace: my-dirived-secret-namespace
spec:
  # References are secrets and configmaps in the same namespace as the DerivedSecret that can be referenced
  # The name will become the field name in the .References variable in the templates
  references:
  - name: myConfigMapReferenceKey
    configMapRef:
      name: my-configmap
      # optional: false
  - name: mySecretReferenceKey
    secretRef:
      name: my-secret
      # optional: false
  # To enforce RBAC, a service account must be specified which has access to the references
  serviceAccountName: my-service-account
  # The type field of the generated secret
  targetType: Opaque
  # By default, the generated secret will share its name with the DerivedSecret
  targetName: my-target-name
  # By default, the generated secret will be in the same namespace as the DerivedSecret
  # The service account specified must also have access to this namespace
  targetNamespace: my-target-namespace
  # data and stringData work as expected, but their values are passed through text/template, with references available under .References.<key>
  # Sprig functions are available
  # Input secrets will need to be passed through b64dec to produce string values, if appropriate, and string outputs to data must be encoded with b64enc
  data:
    foo: |-
      {{ .References.mySecretReferenceKey.data.foo }}
  stringData:
    bar: |-
      {{ .References.myConfigMapReferenceKey.data.bar }}
  # Common use-cases are provided
  # It is an error to provide more than one, or provide one as well as data/stringData
  prefab:
    # If true, copy all data fields from all references (handling configmap data as stringData)
    # It is an error if any reference contains the same key
    copyAll: false
    # If non-empty, copy just the specified keys from the specified references
    # It is an error if the same key is specified in more than one reference
    copyIncluding: []
    # - ref: myConfigMapReferenceKey
    #   # keys: []
    #   # allKeys: true
    # If non-empty, copy everything but the specified keys from the specified references
    # Same schema as copyIncluding
    # It is an error if any specified reference contains the same key
    copyExcluding: []
  
status:
  secretName: my-target-name
  secretNamespace: my-target-namespace
  # error: "error message"
  lastSyncAttempt: <some timestamp>
  lastSync: <some timestamp> 
DerivedConfigmap works identically, but with data/binaryData instead of data/stringData, and it can only reference other ConfigMaps, and doesn't have a type

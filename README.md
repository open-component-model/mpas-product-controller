# MPAS Product Controller

[![REUSE status](https://api.reuse.software/badge/github.com/open-component-model/mpas-product-controller)](https://api.reuse.software/info/github.com/open-component-model/mpas-product-controller)

## About this project

`mpas-product-controller` is the main component for the MPAS system. It reconciles deployments, targets and is responsible for validating configuration
changes made by product authors.

## Functionality

### ProductDescription

A component is considered MPAS ready if it contains a valid `ProductDescription`. A valid product description might look something like this:

```yaml
apiVersion: meta.mpas.ocm.software/v1alpha1
kind: ProductDescription
metadata:
  name: weave-gitops
spec:
  description: Weave GitOps is a powerful extension to Flux, a leading GitOps engine and CNCF project. Weave GitOps provides insights into your application deployments, and makes continuous delivery with GitOps easier to adopt and scale across your teams.
  pipelines:
  - name: wego
    targetRoleName: ingress
    source:
      name: manifests
      version: 1.0.0
    localization:
      name: config
      version: 1.0.0
    configuration:
      rules:
        name: config
        version: 1.0.0
      readme:
        name: instructions
        version: 1.0.0
    validation:
      name: validation
      version: 1.0.0
  targetRoles:
  - name: ingress
    type: kubernetes
    selector:
      matchLabels:
        target.mpas.ocm.software/ingress-enabled: "true"
```

It defines a set of `pipelines` that are executed after each other. A pipeline consists of the following objects:

#### source

Defines the source item from which to fetch the component. This can be either a ComponentVersion or a Snapshot.

#### localization

Localization defines localization rules that will be applied to the component before any configuration can occur.

#### configuration

Configuration contains config rules for the component. It also includes a README which will be put into the final
README of the pull request that is generated later for the product.

#### validation

Validation rules contains rules that are applied to the config object.

#### targetRoles

Target roles defines rules for picking a target to use for deploying this resource.

### ProductDeployment

A `ProductDeployment` is generated using a [ProductDeploymentGenerator](#productdeploymentgenerator). It contains the steps taken from the description and translates it into a reconcileable object. It might looks something like this:

```yaml
apiVersion: mpas.ocm.software/v1alpha1
kind: ProductDeployment
metadata:
  creationTimestamp: null
  name: test-product-deployment
  namespace: test-namespace
spec:
  component:
    name: github.com/open-component-model/mpas
    registry:
      url: https://github.com/open-component-model/source
    version: v0.0.1
  pipelines:
  - configuration:
      rules:
        name: config
        referencePath:
        - name: backend
        version: 1.0.0
    localization:
      name: config
      referencePath:
      - name: backend
      version: 1.0.0
    name: backend
    resource:
      name: manifests
      referencePath:
      - name: backend
      version: 1.0.0
    targetRole:
      selector:
        matchLabels:
          target.mpas.ocm.software/ingress-enabled: "true"
      type: kubernetes
    validation:
      name: validation
      referencePath:
      - name: backend
      version: 1.0.0
  serviceAccountName: test-service-account
status: {}
```

### ProductDeploymentPipelines

Pipelines are an intermediate object that is used to execute each object from a productdeployment.

Each pipeline corresponds to one object and will create the necessary objects to satisfy that pipeline. For example, creating Localization resources.

### ProductDeploymentGenerator

A generator takes a [Subscription](https://github.com/open-component-model/replication-controller#usage) and retrieves the component from the destination location.
It then fetches the product description and generates a [ProductDeployment](#productdeployment) out of it. A `ProductDeploymentGenerator` might look something like this:

```yaml
apiVersion: mpas.ocm.software/v1alpha1
kind: ProductDeploymentGenerator
metadata:
  name: weave-gitops
  namespace: mpas-ocm-applications
spec:
  interval: 1m
  serviceAccountName: mpas-ocm-applications
  subscriptionRef:
    name: weave-gitops-subscription
    namespace: mpas-ocm-applications
```

### Target

A Target defines where the end result should be deployed to. The end result is usually a set of manifests or a tar file; it depends on the pipeline.
But whatever it is, the target will decide where to put it and how to get it there.

A Kubernetes based target might look something like this:

```yaml
apiVersion: mpas.ocm.software/v1alpha1
kind: Target
metadata:
  name: kubernetes-target
  namespace: mpas-system
  labels:
    target.mpas.ocm.software/ingress-enabled: "true" # DO NOT CHANGE. expected by the component for target selection.
spec:
  type: kubernetes
  access:
    targetNamespace: default
```

The labels define the capabilities of this Target. Access is up to the target environment to be defined.

A Kubernetes target may define additional an additional kube config and the target namespace to deploy the endresult into.

### Validation

Validation defines rules for validating configuration objects.

Defintion pending...

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright (20xx-)20xx SAP SE or an SAP affiliate company and <your-project> contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/<your-project>).

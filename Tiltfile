# -*- mode: Starlark -*-

kubectl_cmd = "kubectl"

# verify kubectl command exists
if str(local("command -v " + kubectl_cmd + " || true", quiet = True)) == "":
    fail("Required command '" + kubectl_cmd + "' not found in PATH")

# set defaults
settings = {
    "create_secrets": {
        "enable": True,
        "token": os.getenv("GITHUB_TOKEN", ""),
        "email": os.getenv("GITHUB_EMAIL", ""),
        "user": os.getenv("GITHUB_USER", ""),
    },
    "verification_keys": {},
    "root_certificate_secret": {
        "enable": True,
        "name": "registry-certs",
    },
}

# global settings
tilt_file = "./tilt-settings.yaml" if os.path.exists("./tilt-settings.yaml") else "./tilt-settings.json"
settings.update(read_yaml(
    tilt_file,
    default = {},
))

# Use kustomize to build the install yaml files
install = kustomize('config/default')

# Update the root security group. Tilt requires root access to update the
# running process.
objects = decode_yaml_stream(install)
root_certificate = settings.get("root_certificate_secret")
for o in objects:
    if o.get('kind') == 'Deployment' and o.get('metadata').get('name') == 'mpas-product-controller':
        o['spec']['template']['spec']['securityContext']['runAsNonRoot'] = False
        if root_certificate.get("enable"):
            print('updating mpas-product-controller deployment to add generated certificates')
            o['spec']['template']['spec']['volumes'] = [{'name': 'root-certificate', 'secret': {'secretName': root_certificate.get("name"), 'items': [{'key': 'caFile', 'path': 'ca.pem'}]}}]
            o['spec']['template']['spec']['containers'][0]['volumeMounts'] = [{'mountPath': '/certs', 'name': 'root-certificate'}]
        break

updated_install = encode_yaml_stream(objects)

# Apply the updated yaml to the cluster.
# Allow duplicates so the e2e test can include this tilt file with other tilt files
# setting up the same namespace.
k8s_yaml(updated_install, allow_duplicates = True)

load('ext://restart_process', 'docker_build_with_restart')

# enable hot reloading by doing the following:
# - locally build the whole project
# - create a docker imagine using tilt's hot-swap wrapper
# - push that container to the local tilt registry
# Once done, rebuilding now should be a lot faster since only the relevant
# binary is rebuilt and the hot swat wrapper takes care of the rest.
local_resource(
    'mpas-product-controller-binary',
    'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o bin/manager ./',
    deps = [
        "main.go",
        "go.mod",
        "go.sum",
        "api",
        "controllers",
        "pkg",
        "hack/entrypoint.sh",
    ],
)

# Build the docker image for our controller. We use a specific Dockerfile
# since tilt can't run on a scratch container.
# `only` here is important, otherwise, the container will get updated
# on _any_ file change. We only want to monitor the binary.
# If debugging is enabled, we switch to a different docker file using
# the delve port.
entrypoint = ['/entrypoint.sh', '/manager']
dockerfile = 'tilt.dockerfile'
docker_build_with_restart(
    'ghcr.io/open-component-model/mpas-product-controller',
    '.',
    dockerfile = dockerfile,
    entrypoint = entrypoint,
    only=[
      './bin',
      './hack/entrypoint.sh',
    ],
    live_update = [
        sync('./bin/manager', '/manager'),
        sync('./hack/entrypoint.sh', '/entrypoint.sh'),
    ],
)

//  # Configuration Instructions
//
//  This is the api service of the podinfo microservices application.
//
//  The following parameters are available for configuration:
//
//  | Parameter | Type    | Default          | Description                            |
//  |-----------|---------|------------------|----------------------------------------|
//  | replicas  | integer | 2                | Number of replicas for the application |
//  | cacheAddr | string  | tcp://redis:6379 | Address of the cache server            |

#SchemaVersion: "v1.0.0"

// this field has a default value of 2
replicas: *2 | int
// this is a required field with of type string with a constraint
cacheAddr: *"tcp://redis:6379" | string & =~"^tcp://.+|^https://.+"

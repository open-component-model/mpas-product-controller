// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package cue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaultSchema = `
// This a config file for the product deployment pipeline.
// It is named config.cue.

#SchemaVersion: "v1.0.0"
// this field has a default value of 2
replicas: *2 | int

// this is a required field with of type string with a constraint
redis_url!: string & =~ "^https://"
    
// this is an optional field
labels?: {[string]:string}

// this is a generated field that will not be exposed to in the config.cue file
// part of the final configuration values
max_replicas: replicas * 5 @private(true)
`

var modifiedSchema = `
// This a config file for the product deployment pipeline.
// It is named config.cue.

#SchemaVersion: "v1.0.0"

deployment: {
	// this field has a default value of 2
	replicas: *2 | int
	
	// this is a required field with of type string with a constraint
	redis_url!: *"http://localhost:6379" | string & =~ "^https://"

	// this is an optional field
	labels?: {[string]:string}
	
	// this is a generated field that will not be exposed to in the config.cue file
	// part of the final configuration values
	max_replicas: replicas * 5 @private(true)
}
`

func generateConfigFile(t *testing.T, src string) *File {
	f, err := New("config", "", src)
	require.NoError(t, err)
	return f
}

func TestCue_ValidGeneration(t *testing.T) {
	f := generateConfigFile(t, defaultSchema)

	comments := f.Comments()
	assert.Equal(t, "This a config file for the product deployment pipeline.\nIt is named config.cue.\n", comments)

	version, err := f.SchemaVersion()
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", version)

	form, err := f.Format()
	require.NoError(t, err)
	assert.NotNil(t, form)
}

func TestCue_CopyWithoutPrivateFields(t *testing.T) {
	testCase := []struct {
		name     string
		config   string
		expected func(t *testing.T, f []byte) error
	}{
		{
			name: "valid config using default values",
			config: `
#SchemaVersion: "v1.0.0"
// this field has a default value of 2
replicas: *2 | int
max_replicas: replicas * 5 @private(true)
`,
			expected: func(t *testing.T, f []byte) error {
				str := string(f)
				assert.Contains(t, str, "replicas: *2 | int")
				assert.NotContains(t, str, "max_replicas: replicas * 5 @private(true)")
				return nil
			},
		},
		{
			name: "valid config with nested fields",
			config: `
#SchemaVersion: "v1.0.0"
// this field has a default value of 2
replicas: *2 | int
deployment: {
	// this field has a default value of 2
	replicas: *2 | int
	max_replicas: replicas * 5 @private(true)
}
`,
			expected: func(t *testing.T, f []byte) error {
				str := string(f)
				assert.Contains(t, str, "replicas: *2 | int")
				assert.NotContains(t, str, "max_replicas: replicas * 5 @private(true)")
				return nil
			},
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			config := generateConfigFile(t, tc.config)
			ok := config.ContainsPrivateFields()
			assert.True(t, ok)
			f2 := config.CopyWithoutPrivateFields()

			version, err := f2.SchemaVersion()
			require.NoError(t, err)
			assert.Equal(t, "v1.0.0", version)

			form, err := f2.Format()
			require.NoError(t, err)
			tc.expected(t, form)
		})
	}
}

func TestCue_Merge(t *testing.T) {
	schema := generateConfigFile(t, defaultSchema)
	modifiedSchema := generateConfigFile(t, modifiedSchema)
	testsCases := []struct {
		name      string
		config    string
		schema    *File
		wanted    func(f *File) error
		wantedErr func(err error) error
	}{
		{
			name: "valid config using default values",
			config: `
redis_url: "http://localhost:6379"
`,
			schema: schema,
			wanted: func(f *File) error {
				s, err := f.Yaml()
				require.NoError(t, err)
				str := string(s)
				assert.Contains(t, str, "replicas: 2")
				assert.Contains(t, str, "redis_url: http://localhost:6379")
				assert.Contains(t, str, "labels: {}\n")
				assert.NotContains(t, str, "max_replicas: 10")
				return nil
			},
		},
		{
			name: "valid config",
			config: `
replicas: 3
redis_url: "http://localhost:6379"
labels: {
  "app": "redis"
  "env": "dev"
}
`,
			schema: schema,
			wanted: func(f *File) error {
				s, err := f.Yaml()
				require.NoError(t, err)
				str := string(s)
				assert.Contains(t, str, "replicas: 3")
				assert.Contains(t, str, "redis_url: http://localhost:6379")
				assert.Contains(t, str, "labels:\n  app: redis\n  env: dev\n")
				assert.NotContains(t, str, "max_replicas: 15")
				return nil
			},
		},
		{
			name:   "invalid config empty config",
			config: "",
			schema: schema,
			wantedErr: func(err error) error {
				require.Error(t, err)
				// we can parse an empty cue file just fine
				// but validation will fails because we require concrete values
				assert.Contains(t, err.Error(), "redis_url")
				return nil
			},
		},
		{
			name: "invalid config missing a required field",
			config: `
replicas: 3
labels: {
	"app": "redis"
	"env": "dev"
}
`,
			schema: schema,
			wantedErr: func(err error) error {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "redis_url")
				return nil
			},
		},
		{
			name: "valid config using modified schema",
			config: `
replicas: 3
redis_url: "http://localhost:6379"
`,
			schema: modifiedSchema,
			wanted: func(f *File) error {
				s, err := f.Yaml()
				require.NoError(t, err)
				str := string(s)
				assert.Contains(t, str, "replicas: 3")
				assert.Contains(t, str, "redis_url: http://localhost:6379")
				assert.NotContains(t, str, "max_replicas: 10")
				assert.Contains(t, str, "deployment:\n  replicas: 2\n  redis_url: http://localhost:6379\n  labels: {}\n")
				return nil
			},
		},
	}

	for _, tc := range testsCases {
		t.Run(tc.name, func(t *testing.T) {
			config := generateConfigFile(t, tc.config)
			merged, err := config.Merge(tc.schema, false)
			require.NoError(t, err)

			v := merged.value()
			assert.NotNil(t, v)

			err = merged.Vet()
			if tc.wantedErr != nil {
				err = tc.wantedErr(err)
				require.NoError(t, err)
				return
			}
			require.NoError(t, err)

			_, err = merged.value().MarshalJSON()
			require.NoError(t, err)

			err = tc.wanted(merged)
			require.NoError(t, err)
		})
	}
}

func TestCue_Unify(t *testing.T) {
	schema := generateConfigFile(t, defaultSchema)
	schema2 := generateConfigFile(t, `
deploymentReplicas: 3
#Deployment: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	replicas:   deploymentReplicas
}
deployment: #Deployment
`)
	schema3 := generateConfigFile(t, `
#Deployment: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	...
}
redis_url: "https://localhost:6379"
deployment: #Deployment
`)

	resultYaml := `---
replicas: 2
deploymentReplicas: 3
redis_url: https://localhost:6379
max_replicas: 10
deployment:
  apiVersion: apps/v1
  kind: Deployment
  replicas: 3
`
	unified, err := schema.Unify([]*File{schema2, schema3})
	require.NoError(t, err)

	data, err := unified.Yaml()
	require.NoError(t, err)

	assert.Equal(t, resultYaml, string(data))
}

func TestCue_Validate(t *testing.T) {
	schema := generateConfigFile(t, defaultSchema)
	testsCases := []struct {
		name      string
		config    string
		wantedErr func(err error) error
	}{
		{
			name: "valid config using default values",
			config: `
redis_url: "https://localhost:6379"
`,
		},
		{
			name: "valid config",
			config: `
replicas: 3
redis_url: "https://localhost:6379"
labels: {
	"app": "redis"
	"env": "dev"
}
message?: string
`,
		},
		{
			name: "invalid config empty config",
			config: `
`,
			wantedErr: func(err error) error {
				require.Error(t, err)
				// we can parse an empty cue file just fine
				// but validation will fails because we require concrete values
				assert.Contains(t, err.Error(), "redis_url")
				return nil
			},
		},
		{
			name: "invalid config with wrong type",
			config: `
replicas: "3"
redis_url: "https://localhost:6379"
labels: {
	"app": "redis"
	"env": "dev"
}
`,
			wantedErr: func(err error) error {
				require.Error(t, err)
				// replicas should be an int
				assert.Contains(t, err.Error(), "replicas")
				return nil
			},
		},
		{
			name: "valid using same schema constraints",
			config: `
replicas: *2 | int
// this is a required field with of type string with a constraint
redis_url: "https://localhost:6379"
    
// this is an optional field
labels?: {[string]:string}
`,
		},
	}

	for _, tc := range testsCases {
		t.Run(tc.name, func(t *testing.T) {
			config := generateConfigFile(t, tc.config)
			merged, err := config.Merge(schema, false)
			require.NoError(t, err)
			err = merged.Validate(schema)
			if tc.wantedErr != nil {
				err = tc.wantedErr(err)
				require.NoError(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

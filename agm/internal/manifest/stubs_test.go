package manifest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrite_Stub(t *testing.T) {
	m := &Manifest{
		SchemaVersion: "2.0",
		SessionID:     "test",
		Name:          "test",
	}
	err := Write("/tmp/test-manifest.yaml", m)
	assert.NoError(t, err, "Write stub should return nil")
}

func TestRead_Stub(t *testing.T) {
	m, err := Read("/tmp/nonexistent.yaml")
	assert.Nil(t, m, "Read stub should return nil manifest")
	assert.ErrorIs(t, err, os.ErrNotExist, "Read stub should return os.ErrNotExist")
}

func TestList_Stub(t *testing.T) {
	list, err := List("/tmp/sessions")
	assert.Nil(t, list, "List stub should return nil")
	assert.ErrorIs(t, err, ErrYAMLBackendRemoved, "List stub should return ErrYAMLBackendRemoved")
}

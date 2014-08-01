package wolfram

import (
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

const apiKey = "A6393P-2Y4J5AA4UW"

func TestQuery(t *testing.T) {
	api := NewApi(apiKey, "Russia")
	res, err := api.Query("rouble")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, true, res.IsSuccess)
	assert.Equal(t, false, res.IsError)
	assert.NotEmpty(t, res.Pods)
	for _, pod := range res.Pods {
		log.Printf("Pod %s:\n%s\n\n", pod.Title, pod.SubPods[0].Plaintext)
	}
}

package wikipedia

import (
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueryRU(t *testing.T) {
	api := NewApi()
	query := "лопата"
	res, err := api.Query(RU, query)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, query, res.Query)
	assert.NotEmpty(t, res.Items)
	for _, it := range res.Items {
		log.Printf("Item %s:\n%s (%s)\n\n", it.Text, it.Description, it.URL)
	}
}

func TestQueryEN(t *testing.T) {
	api := NewApi()
	query := "shovel"
	res, err := api.Query(EN, query)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, query, res.Query)
	assert.NotEmpty(t, res.Items)
	for _, it := range res.Items {
		log.Printf("Item %s:\n%s (%s)\n\n", it.Text, it.Description, it.URL)
	}
}

package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

type test struct {
	input prometheusCredentials
	want  string
}

func TestGeneratePrometheusURL(t *testing.T) {
	tests := []test{
		{
			input: prometheusCredentials{
				URL:      "http://prometheus",
				User:     "test",
				Password: "test",
			},
			want: "http://test:test@prometheus",
		},
		{
			input: prometheusCredentials{
				URL:      "https://prometheus",
				User:     "test",
				Password: "test",
			},
			want: "https://test:test@prometheus",
		},
		{
			input: prometheusCredentials{
				URL:      "prometheus",
				User:     "test",
				Password: "test",
			},
			want: "https://test:test@prometheus",
		},
		{
			input: prometheusCredentials{
				URL:      "  prometheus",
				User:     "test",
				Password: "test",
			},
			want: "https://test:test@prometheus",
		},
		{
			input: prometheusCredentials{
				URL:      "prometheus",
				User:     "",
				Password: "test",
			},
			want: "https://prometheus",
		},
	}
	for _, test := range tests {
		url := generatePrometheusURL(&test.input)
		assert.EqualValues(t, test.want, url)
	}
}

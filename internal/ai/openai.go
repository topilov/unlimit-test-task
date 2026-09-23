package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"apm-investigator/internal/contract"
	"apm-investigator/internal/incident"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

const (
	maxAttempts     = 2
	retryDelay      = 200 * time.Millisecond
	maxOutputTokens = 4000
)

type OpenAI struct {
	client        openai.Client
	model, effort string
	timeout       time.Duration
	resources     contract.Resources
}

func NewOpenAI(key, model, effort string, timeout time.Duration, opts ...option.RequestOption) (*OpenAI, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("live mode requires OPENAI_API_KEY; mock mode needs no key")
	}
	if model == "" || timeout <= 0 {
		return nil, incident.ErrInvalidInput
	}
	options := []option.RequestOption{option.WithAPIKey(key)}
	options = append(options, opts...)
	options = append(options, option.WithMaxRetries(0))
	resources, err := contract.Load()
	if err != nil {
		return nil, err
	}
	return &OpenAI{
		resources: resources,
		client:    openai.NewClient(options...),
		model:     model,
		effort:    effort,
		timeout:   timeout,
	}, nil
}
func (a *OpenAI) Next(ctx context.Context, input Input) (turn incident.Turn, meta incident.Metadata, err error) {
	params, err := a.request(input)
	if err != nil {
		return turn, meta, err
	}
	response, meta, err := a.respond(ctx, params)
	if err != nil {
		return turn, meta, err
	}
	turn, err = parseResponse(response)
	return turn, meta, err
}

func (a *OpenAI) respond(ctx context.Context, params responses.ResponseNewParams) (response *responses.Response, meta incident.Metadata, err error) {
	meta.Model = a.model
	meta.ReasoningEffort = a.effort
	started := time.Now()
	defer func() { meta.DurationMS = time.Since(started).Milliseconds() }()
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		meta.Attempts = attempt
		response, err = a.client.Responses.New(ctx, params)
		if err == nil {
			break
		}
		if attempt == maxAttempts || !transient(err) || ctx.Err() != nil {
			return nil, meta, modelError(err)
		}
		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, meta, fmt.Errorf("%w: deadline/cancellation", incident.ErrModelUnavailable)
		case <-timer.C:
		}
	}
	meta.ResponseID = response.ID
	meta.InputTokens = response.Usage.InputTokens
	meta.OutputTokens = response.Usage.OutputTokens
	meta.CachedTokens = response.Usage.InputTokensDetails.CachedTokens
	return response, meta, nil
}

func transient(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
func modelError(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf("%w: OpenAI HTTP %d", incident.ErrModelUnavailable, apiErr.StatusCode)
	}
	return fmt.Errorf("%w: transport or deadline failure", incident.ErrModelUnavailable)
}

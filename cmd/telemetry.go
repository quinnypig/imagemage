package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"imagemage/pkg/imagegen"
	"os"
	"time"
)

// No telemetry leaves the CLI unless the operator supplies an OTLP destination.
func initializeTelemetry() func() {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return func() {}
	}
	exporter, err := otlptracehttp.New(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "Telemetry initialization failed")
		return func() {}
	}
	name := os.Getenv("OTEL_SERVICE_NAME")
	if name == "" {
		name = "imagemage"
	}
	res, _ := resource.New(context.Background(), resource.WithFromEnv(), resource.WithAttributes(attribute.String("service.name", name)))
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(provider)
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if provider.Shutdown(ctx) != nil {
			fmt.Fprintln(os.Stderr, "Telemetry flush incomplete")
		}
	}
}

type tracedImageClient struct {
	client   imagegen.Client
	provider imagegen.Provider
	model    string
}

func (c *tracedImageClient) Generate(ctx context.Context, req imagegen.Request) (imagegen.Result, error) {
	return c.call(ctx, req, false)
}
func (c *tracedImageClient) Edit(ctx context.Context, req imagegen.Request) (imagegen.Result, error) {
	return c.call(ctx, req, true)
}
func (c *tracedImageClient) call(ctx context.Context, req imagegen.Request, edit bool) (imagegen.Result, error) {
	provider := string(c.provider)
	if provider == "gemini" {
		provider = "gcp.gemini"
	}
	ctx, span := otel.Tracer("imagemage.ai").Start(ctx, "generate_content "+c.model, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	span.SetAttributes(attribute.String("gen_ai.operation.name", "generate_content"), attribute.String("gen_ai.provider.name", provider), attribute.String("gen_ai.request.model", c.model), attribute.String("app.cost.scope", "model_call"), attribute.String("app.cost.pricing_status", "no_usage_reported"), attribute.Bool("image.edit", edit))
	captureImageContent(span, "request", map[string]any{"prompt": req.Prompt, "image_count": len(req.Images), "quality": req.Quality, "resolution": req.Resolution, "aspect_ratio": req.AspectRatio})
	var result imagegen.Result
	var err error
	if edit {
		result, err = c.client.Edit(ctx, req)
	} else {
		result, err = c.client.Generate(ctx, req)
	}
	if err != nil {
		span.SetStatus(codes.Error, "")
		span.SetAttributes(attribute.String("error.type", "generation_failed"))
		return result, err
	}
	if result.Model != "" {
		span.SetAttributes(attribute.String("gen_ai.response.model", result.Model))
	}
	captureImageContent(span, "response", map[string]any{"suggested_name": result.SuggestedName, "image_returned": result.ImageData != ""})
	return result, nil
}

func captureImageContent(span trace.Span, direction string, value any) {
	if !span.IsRecording() || os.Getenv("OTEL_GENAI_CAPTURE_CONTENT") == "false" {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	content := []rune(string(data))
	size := len(content)
	if size > 16384 {
		content = content[:16384]
	}
	key := "app.ai." + direction
	span.SetAttributes(attribute.String(key+".content", string(content)), attribute.Int(key+".content_length", size), attribute.Bool(key+".content_truncated", size > 16384))
}

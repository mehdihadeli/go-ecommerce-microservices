package tracing

// https://opentelemetry.io/docs/reference/specification/
// https://opentelemetry.io/docs/instrumentation/go/getting-started/
// https://opentelemetry.io/docs/instrumentation/go/manual/
// https://opentelemetry.io/docs/reference/specification/trace/semantic_conventions/
// https://uptrace.dev/opentelemetry/go-tracing.html
// https://lightstep.com/blog/opentelemetry-go-all-you-need-to-know
// https://trstringer.com/otel-part2-instrumentation/
// https://trstringer.com/otel-part5-propagation/
// https://github.com/tedsuo/otel-go-basics/blob/main/server.go
// https://github.com/riferrei/otel-with-golang/blob/main/main.go

import (
	"context"
	"os"
	"time"

	"github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/config/environemnt"

	"emperror.dev/errors"
	"github.com/samber/lo"
	"go.opentelemetry.io/contrib/propagators/ot"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

type TracingOpenTelemetry struct {
	config      *TracingOptions
	environment environemnt.Environment
	appTracer   AppTracer
	provider    *tracesdk.TracerProvider
}

// Create one tracer per package
// NOTE: You only need a tracer if you are creating your own spans

func NewOtelTracing(
	config *TracingOptions,
	environment environemnt.Environment,
) (*TracingOpenTelemetry, error) {
	otelTracing := &TracingOpenTelemetry{
		config:      config,
		environment: environment,
	}

	resource, err := otelTracing.newResource()
	if err != nil {
		return nil, errors.WrapIf(err, "failed to create resource")
	}

	appTracer, err := otelTracing.initTracer(resource)
	if err != nil {
		return nil, err
	}

	otelTracing.appTracer = appTracer

	return otelTracing, nil
}

func (o *TracingOpenTelemetry) Shutdown(ctx context.Context) error {
	return o.provider.Shutdown(ctx)
}

func (o *TracingOpenTelemetry) newResource() (*resource.Resource, error) {
	// https://github.com/uptrace/uptrace-go/blob/master/example/otlp-traces/main.go#L49C1-L56C5
	resource, err := resource.New(context.Background(),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceName(o.config.ServiceName),
			semconv.ServiceVersion(o.config.Version),
			attribute.Int64("ID", o.config.Id),
			attribute.String("environment", o.environment.GetEnvironmentName()),
			semconv.TelemetrySDKVersionKey.String("v1.21.0"), // semconv version
			semconv.TelemetrySDKLanguageGo,
		))

	return resource, err
}

func (o *TracingOpenTelemetry) initTracer(
	resource *resource.Resource,
) (AppTracer, error) {
	exporters, err := o.configExporters()
	if err != nil {
		return nil, err
	}

	var sampler tracesdk.Sampler
	if o.config.AlwaysOnSampler {
		sampler = tracesdk.AlwaysSample()
	} else {
		sampler = tracesdk.NeverSample()
	}

	batchExporters := lo.Map(
		exporters,
		func(item tracesdk.SpanExporter, index int) tracesdk.TracerProviderOption {
			return tracesdk.WithBatcher(item)
		},
	)

	// https://opentelemetry.io/docs/instrumentation/go/exporting_data/#resources
	// Resources are a special type of attribute that apply to all spans generated by a process
	opts := append(
		batchExporters,
		tracesdk.WithResource(resource),
		tracesdk.WithSampler(sampler),
	)

	provider := tracesdk.NewTracerProvider(opts...)

	// Register our TracerProvider as the global so any imported
	// instrumentation in the future will default to using it.
	otel.SetTracerProvider(provider)
	o.provider = provider

	// https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/propagators/ot/ot_propagator.go
	// https://github.com/open-telemetry/opentelemetry-go/blob/main/propagation/trace_context.go
	// https://github.com/open-telemetry/opentelemetry-go/blob/main/propagation/baggage.go/
	// https://trstringer.com/otel-part5-propagation/
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			ot.OT{}, // should be placed before `TraceContext` for preventing conflict
			propagation.Baggage{},
			propagation.TraceContext{},
		),
	)

	// https://trstringer.com/otel-part2-instrumentation/
	// Finally, set the tracer that can be used for this package. global app tracer
	appTracer := NewAppTracer(o.config.InstrumentationName)

	return appTracer, nil
}

func (o *TracingOpenTelemetry) configExporters() ([]tracesdk.SpanExporter, error) {
	ctx := context.Background()
	traceOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithTimeout(5 * time.Second),
		otlptracegrpc.WithInsecure(),
	}

	var exporters []tracesdk.SpanExporter

	if !o.config.UseOTLP { //nolint:nestif

		// jaeger exporter removed from otel spec (it used jaeger agent and jaeger agent port), now we should use OTLP which supports by jaeger now by its built-in `collector`
		// https://medium.com/jaegertracing/introducing-native-support-for-opentelemetry-in-jaeger-eb661be8183c
		// https://www.jaegertracing.io/docs/1.38/apis/#opentelemetry-protocol-stable
		// https://deploy-preview-1892--opentelemetry.netlify.app/blog/2022/jaeger-native-otlp/
		// https://www.jaegertracing.io/docs/1.49/getting-started/
		// https://opentelemetry.io/docs/instrumentation/go/exporters/
		// https://opentelemetry.io/docs/specs/otlp/
		// https://github.com/open-telemetry/opentelemetry-go/pull/4467
		if o.config.JaegerExporterOptions != nil {
			traceOpts = append(
				traceOpts,
				otlptracegrpc.WithEndpoint(
					o.config.JaegerExporterOptions.OTLPEndpoint,
				),
				otlptracegrpc.WithHeaders(
					o.config.JaegerExporterOptions.OTLPHeaders,
				),
			)

			// send otel traces to jaeger builtin collector endpoint (default grpc port: 4317)
			// https://opentelemetry.io/docs/collector/
			jaegerTraceExporter, err := otlptracegrpc.New(ctx, traceOpts...)
			if err != nil {
				return nil, errors.WrapIf(
					err,
					"failed to create oltptrace exporter for jaeger",
				)
			}

			exporters = append(exporters, jaegerTraceExporter)
		}

		if o.config.ZipkinExporterOptions != nil {
			zipkinExporter, err := zipkin.New(
				o.config.ZipkinExporterOptions.Url,
			)
			if err != nil {
				return nil, errors.WrapIf(
					err,
					"failed to create exporter for zipkin",
				)
			}

			exporters = append(exporters, zipkinExporter)
		}

		if o.config.ElasticApmExporterOptions != nil {
			// https://www.elastic.co/guide/en/apm/guide/current/open-telemetry.html
			// https://www.elastic.co/guide/en/apm/guide/current/open-telemetry-direct.html#instrument-apps-otel
			// https://github.com/anilsenay/go-opentelemetry-examples/blob/elastic/cmd/main.go#L35
			traceOpts = append(
				traceOpts,
				otlptracegrpc.WithEndpoint(
					o.config.ElasticApmExporterOptions.OTLPEndpoint,
				),
				otlptracegrpc.WithHeaders(
					o.config.ElasticApmExporterOptions.OTLPHeaders,
				),
			)

			// send otel traces to jaeger builtin collector endpoint (default grpc port: 4317)
			// https://opentelemetry.io/docs/collector/
			elasticApmExporter, err := otlptracegrpc.New(ctx, traceOpts...)
			if err != nil {
				return nil, errors.WrapIf(
					err,
					"failed to create oltptrace exporter for elastic-apm",
				)
			}

			exporters = append(exporters, elasticApmExporter)
		}

		if o.config.UptraceExporterOptions != nil {
			// https://github.com/uptrace/uptrace-go/blob/master/example/otlp-traces/main.go#L49C1-L56C5
			// https://uptrace.dev/get/opentelemetry-go.html#exporting-traces
			// https://uptrace.dev/get/opentelemetry-go.html#exporting-metrics
			traceOpts = append(
				traceOpts,
				otlptracegrpc.WithEndpoint(
					o.config.UptraceExporterOptions.OTLPEndpoint,
				),
				otlptracegrpc.WithHeaders(
					o.config.UptraceExporterOptions.OTLPHeaders,
				),
			)

			// send otel traces to jaeger builtin collector endpoint (default grpc port: 4317)
			// https://opentelemetry.io/docs/collector/
			uptraceExporter, err := otlptracegrpc.New(ctx, traceOpts...)
			if err != nil {
				return nil, errors.WrapIf(
					err,
					"failed to create oltptrace exporter for uptrace",
				)
			}

			exporters = append(exporters, uptraceExporter)
		}

		if o.config.UseStdout {
			stdExporter, err := stdouttrace.New(
				stdouttrace.WithWriter(
					os.Stdout,
				), // stdExporter default is `stdouttrace.WithWriter(os.Stdout)`, we can remove this also
				stdouttrace.WithPrettyPrint(), // make output json with pretty printing
			)
			if err != nil {
				return nil, errors.WrapIf(err, "creating stdout exporter")
			}

			exporters = append(exporters, stdExporter)
		}
	} else {
		// use some otel collector endpoints
		for _, oltpProvider := range o.config.OTLPProviders {
			if !oltpProvider.Enabled {
				continue
			}

			traceOpts = append(traceOpts, otlptracegrpc.WithEndpoint(oltpProvider.OTLPEndpoint), otlptracegrpc.WithHeaders(oltpProvider.OTLPHeaders))

			// send otel metrics to an otel collector endpoint (default grpc port: 4317)
			// https://opentelemetry.io/docs/collector/
			// https://github.com/uptrace/uptrace-go/blob/master/example/otlp-traces/main.go#L29
			// https://github.com/open-telemetry/opentelemetry-go/blob/main/exporters/otlp/otlptrace/otlptracehttp/example_test.go#L70
			traceExporter, err := otlptracegrpc.New(ctx, traceOpts...)
			if err != nil {
				return nil, errors.WrapIf(err, "failed to create otlptracegrpc exporter")
			}

			exporters = append(exporters, traceExporter)
		}
	}

	return exporters, nil
}

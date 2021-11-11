package xk6_client_tracing

import (
	"context"
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/jaegerexporter"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/model/pdata"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func init() {
	modules.Register("k6/x/tracing", new(ClientTracing))
}

type exporter string

const (
	noExporter exporter = ""
	// todo: add http
	otlpExporter exporter = "otlp"
	// todo: add thrift, http
	jaegerExporter exporter = "jaeger"
)

type protocol string

const (
	httpProtocol   protocol = "http"
	grpcProtocol   protocol = "grpc"
	thriftProtocol protocol = "thrift"
)

type Config struct {
	Exporter exporter `json:"type"`
	Protocol protocol `json:"protocol"`
	Endpoint string   `json:"url"`
}

type ClientTracing struct {
	exporter consumer.Traces
	cfg      *Config
}

func (c *ClientTracing) XClient(ctxPtr *context.Context, cfg Config) interface{} {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "0.0.0.0:4317"
	}

	var (
		factory     component.ExporterFactory
		exporterCfg config.Exporter
	)
	switch cfg.Exporter {
	case noExporter, otlpExporter:
		factory = otlpexporter.NewFactory()
		exporterCfg = factory.CreateDefaultConfig()
		exporterCfg.(*otlpexporter.Config).GRPCClientSettings = configgrpc.GRPCClientSettings{
			Endpoint: cfg.Endpoint,
			TLSSetting: configtls.TLSClientSetting{
				Insecure: true,
			},
		}
	case jaegerExporter:
		factory = jaegerexporter.NewFactory()
		exporterCfg = factory.CreateDefaultConfig()
		exporterCfg.(*jaegerexporter.Config).GRPCClientSettings = configgrpc.GRPCClientSettings{
			Endpoint: cfg.Endpoint,
			TLSSetting: configtls.TLSClientSetting{
				Insecure: true,
			},
		}
	default:
		return fmt.Errorf("failed to init exporter: unknown exporter type %s", cfg.Exporter)
	}

	exporter, err := factory.CreateTracesExporter(
		context.Background(),
		component.ExporterCreateSettings{
			TelemetrySettings: component.TelemetrySettings{
				Logger:         zap.NewNop(),
				TracerProvider: trace.NewNoopTracerProvider(),
				MeterProvider:  metric.NewNoopMeterProvider(),
			},
			BuildInfo: component.NewDefaultBuildInfo(),
		},
		exporterCfg,
	)
	if err != nil {
		return err
	}
	exporter.Start(context.Background(), componenttest.NewNopHost())

	if err != nil {
		return fmt.Errorf("failed to create exporter: %v", err)
	}

	c.exporter = exporter
	c.cfg = &cfg

	rt := common.GetRuntime(*ctxPtr)
	return common.Bind(rt, c, ctxPtr)
}

func (c *ClientTracing) Send(ctx context.Context, spans []Span) error {
	resource := pdata.NewResource()

	traces := pdata.NewTraces()
	rspans := traces.ResourceSpans().AppendEmpty()
	resource.CopyTo(rspans.Resource())
	ispans := rspans.InstrumentationLibrarySpans().AppendEmpty()
	for _, span := range spans {
		span.construct().CopyTo(ispans.Spans().AppendEmpty())
	}

	err := c.exporter.ConsumeTraces(ctx, traces)
	if err != nil {
		return err
	}
	return nil
}

package e2e

import (
    "context"
    "fmt"
    "net/http"
    "testing"
    "time"

    "github.com/labstack/echo/v4"
    mediatr2 "github.com/mehdihadeli/go-mediatr"
    "github.com/stretchr/testify/suite"

    "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/logger"

    "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/internal/products/data/uow"
    "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/internal/products/delivery"

    "github.com/stretchr/testify/require"

    "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/constants"
    grpcServer "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/grpc"
    customEcho "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/http/custom_echo"
    defaultLogger "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/logger/default_logger"
    rabbitmqConfigurations "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/rabbitmq/configurations"
    "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/test/containers/testcontainer/rabbitmq"
    webWoker "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/web"

    "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/config"
    "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/internal/products/configurations/mappings"
    "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/internal/products/configurations/mediatr"
    rabbitmq2 "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/internal/products/configurations/rabbitmq"
    "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/internal/products/data/repositories"
    "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/internal/shared/configurations/catalogs/metrics"
    "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/internal/shared/configurations/infrastructure"
    "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/internal/shared/web/workers"
)

type E2ETestSharedFixture struct {
	Cfg *config.Config
	Log logger.Logger
	suite.Suite
}

type E2ETestFixture struct {
	GrpcServer grpcServer.GrpcServer
	HttpServer customEcho.EchoHttpServer
	*delivery.ProductEndpointBase
	workersRunner *webWoker.WorkersRunner
	Ctx           context.Context
	cancel        context.CancelFunc
}

func NewE2ETestSharedFixture(t *testing.T) *E2ETestSharedFixture {
	// we could use EmptyLogger if we don't want to log anything
	log := defaultLogger.Logger
	cfg, _ := config.InitConfig(constants.Test)

	err := mappings.ConfigureProductsMappings()
	if err != nil {
		require.FailNow(t, err.Error())
	}
	require.NoError(t, err)

	integration := &E2ETestSharedFixture{
		Cfg: cfg,
		Log: log,
	}

	return integration
}

func NewE2ETestFixture(shared *E2ETestSharedFixture) *E2ETestFixture {
	ctx, cancel := context.WithCancel(context.Background())

	// we could use EmptyLogger if we don't want to log anything
	c := infrastructure.NewTestInfrastructureConfigurator(shared.T(), shared.Log, shared.Cfg)
	infrastructures, cleanup, err := c.ConfigInfrastructures(ctx)
	if err != nil {
		cancel()
		return nil
	}

	productRep := repositories.NewPostgresProductRepository(infrastructures.Log, infrastructures.Gorm)
	catalogUnitOfWork := uow.NewCatalogsUnitOfWork(infrastructures.Log, infrastructures.Gorm)

	mqBus, err := rabbitmq.NewRabbitMQTestContainers().Start(ctx, shared.T(), func(builder rabbitmqConfigurations.RabbitMQConfigurationBuilder) {
		// Products RabbitMQ configuration
		rabbitmq2.ConfigProductsRabbitMQ(builder)
	})
	if err != nil {
		cancel()
		require.FailNow(shared.T(), err.Error())
	}

	catalogsMetrics, err := metrics.ConfigCatalogsMetrics(infrastructures.Cfg, infrastructures.Metrics)
	if err != nil {
		cancel()
		require.FailNow(shared.T(), err.Error())
	}

	err = mediatr.ConfigProductsMediator(catalogUnitOfWork, productRep, infrastructures, mqBus)
	if err != nil {
		cancel()
		require.FailNow(shared.T(), err.Error())
	}

	grpcServer := grpcServer.NewGrpcServer(infrastructures.Cfg.GRPC, defaultLogger.Logger, infrastructures.Cfg.ServiceName, infrastructures.Metrics)
	httpServer := customEcho.NewEchoHttpServer(infrastructures.Cfg.Http, defaultLogger.Logger, infrastructures.Cfg.ServiceName, infrastructures.Metrics)
	httpServer.SetupDefaultMiddlewares()

	var productEndpointBase *delivery.ProductEndpointBase

	httpServer.RouteBuilder().RegisterGroupFunc("/api/v1", func(v1 *echo.Group) {
		group := v1.Group("/products")
		productEndpointBase = delivery.NewProductEndpointBase(infrastructures, group, mqBus, catalogsMetrics)
	})

	httpServer.RouteBuilder().RegisterRoutes(func(e *echo.Echo) {
		e.GET("", func(ec echo.Context) error {
			return ec.String(http.StatusOK, fmt.Sprintf("%s is running...", infrastructures.Cfg.GetMicroserviceNameUpper()))
		})
	})

	workersRunner := webWoker.NewWorkersRunner([]webWoker.Worker{
		workers.NewRabbitMQWorker(infrastructures.Log, mqBus),
	})

	shared.T().Cleanup(func() {
		// with Cancel() we send signal to done() channel to stop grpc, http and workers gracefully
		// https://dev.to/mcaci/how-to-use-the-context-done-method-in-go-22me
		// https://www.digitalocean.com/community/tutorials/how-to-use-contexts-in-go
		mediatr2.ClearRequestRegistrations()
		cancel()
		cleanup()
	})

	return &E2ETestFixture{
		GrpcServer:          grpcServer,
		HttpServer:          httpServer,
		ProductEndpointBase: productEndpointBase,
		workersRunner:       workersRunner,
		Ctx:                 ctx,
		cancel:              cancel,
	}
}

func (e *E2ETestFixture) Run() {
	go func() {
		if err := e.GrpcServer.RunGrpcServer(e.Ctx, nil); err != nil {
			e.Log.Errorf("(s.RunGrpcServer) err: %v", err)
		}
	}()

	go func() {
		if err := e.HttpServer.RunHttpServer(e.Ctx, nil); err != nil {
			e.Log.Errorf("(s.RunHttpServer) err: %v", err)
		}
	}()

	workersErr := e.workersRunner.Start(e.Ctx)
	go func() {
		for {
			select {
			case _ = <-workersErr:
				e.cancel()
				return
			}
		}
	}()

	// wait for consumers ready to consume before publishing messages, preparation background workers takes a bit time (for preventing messages lost)
	time.Sleep(1 * time.Second)
}
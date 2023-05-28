package orders

import (
	"context"
	"fmt"
	"net/http"

	"emperror.dev/errors"
	"github.com/labstack/echo/v4"

	grpcServer "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/grpc"
	customEcho "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/http/custom_echo"
	subscriptionAll "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/orders/internal/shared/configurations/orders/subscription_all"

	"github.com/mehdihadeli/go-ecommerce-microservices/internal/services/orders/internal/orders/configurations"
	metrics2 "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/orders/internal/shared/configurations/orders/metrics"
	"github.com/mehdihadeli/go-ecommerce-microservices/internal/services/orders/internal/shared/configurations/orders/rabbitmq"
	"github.com/mehdihadeli/go-ecommerce-microservices/internal/services/orders/internal/shared/contracts"
)

type ordersServiceConfigurator struct {
	*contracts.InfrastructureConfigurations
}

func NewOrdersServiceConfigurator(infrastructureConfiguration *contracts.InfrastructureConfigurations) contracts.OrdersServiceConfigurator {
	return &ordersServiceConfigurator{InfrastructureConfigurations: infrastructureConfiguration}
}

func (c *ordersServiceConfigurator) ConfigureOrdersService(ctx context.Context) (*contracts.OrderServiceConfigurations, error) {
	ordersServiceConfigurations := &contracts.OrderServiceConfigurations{}

	ordersServiceConfigurations.OrdersGrpcServer = grpcServer.NewGrpcServer(c.Cfg.GRPC, c.Log, c.Cfg.ServiceName, c.Metrics)
	ordersServiceConfigurations.OrdersEchoServer = customEcho.NewEchoHttpServer(c.Cfg.Http, c.Log, c.Cfg.ServiceName, c.Metrics)
	ordersServiceConfigurations.OrdersEchoServer.SetupDefaultMiddlewares()

	ordersServiceConfigurations.OrdersEchoServer.RouteBuilder().RegisterRoutes(func(e *echo.Echo) {
		e.GET("", func(ec echo.Context) error {
			return ec.String(http.StatusOK, fmt.Sprintf("%s is running...", c.Cfg.GetMicroserviceNameUpper()))
		})
	})

	// Orders Swagger Configs
	c.configSwagger(ordersServiceConfigurations.OrdersEchoServer.RouteBuilder())

	// Orders Metrics Configs
	ordersMetrics, err := metrics2.ConfigOrdersMetrics(c.Cfg, c.Metrics)
	if err != nil {
		return nil, err
	}
	ordersServiceConfigurations.OrdersMetrics = ordersMetrics

	// Orders RabbitMQ Configs
	bus, err := rabbitmq.ConfigOrdersRabbitMQ(ctx, c.Cfg.RabbitMQ, c.InfrastructureConfigurations)
	if err != nil {
		return nil, err
	}
	ordersServiceConfigurations.OrdersBus = bus

	// Orders SubscriptionsAll Configs
	esdbWorker, err := subscriptionAll.ConfigOrdersSubscriptionAllWorker(c.InfrastructureConfigurations, bus)
	if err != nil {
		return nil, err
	}
	ordersServiceConfigurations.OrdersSubscriptionAllWorker = esdbWorker

	// Orders Product Module Configs
	pc := configurations.NewOrdersModuleConfigurator(c.InfrastructureConfigurations, ordersMetrics, bus, ordersServiceConfigurations.OrdersEchoServer.RouteBuilder(), ordersServiceConfigurations.OrdersGrpcServer.GrpcServiceBuilder())
	err = pc.ConfigureOrdersModule(ctx)
	if err != nil {
		return nil, errors.WithMessage(err, "[ordersServiceConfigurator_ConfigureOrdersService.NewOrdersModuleConfigurator] error in order module configurator")
	}

	return ordersServiceConfigurations, nil
}
package catalogs

import (
	"github.com/labstack/echo/v4"
	customEcho "github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/http/custom_echo"
	echoSwagger "github.com/swaggo/echo-swagger"

	"github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/write_service/docs"
)

func (c *catalogsServiceConfigurator) configSwagger(routeBuilder *customEcho.RouteBuilder) {
	//https://github.com/swaggo/swag#how-to-use-it-with-gin
	docs.SwaggerInfo.Version = "1.0"
	docs.SwaggerInfo.Title = "Catalogs Write-Service Api"
	docs.SwaggerInfo.Description = "Catalogs Write-Service Api."

	routeBuilder.RegisterRoutes(func(e *echo.Echo) {
		e.GET("/swagger/*", echoSwagger.WrapHandler)
	})
}
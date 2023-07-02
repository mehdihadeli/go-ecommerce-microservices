package test

import (
	"github.com/spf13/viper"
	"go.uber.org/fx/fxtest"

	"github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/constants"
	"github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/fxapp/contracts"
	"github.com/mehdihadeli/go-ecommerce-microservices/internal/pkg/fxapp/test"
	constants2 "github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogs/read_service/internal/shared/constants"
)

type CatalogsReadTestApplicationBuilder struct {
	contracts.ApplicationBuilder
	tb fxtest.TB
}

func NewCatalogsReadTestApplicationBuilder(tb fxtest.TB) *CatalogsReadTestApplicationBuilder {
	// set viper internal registry
	viper.Set(constants.PROJECT_NAME_ENV, constants2.PROJECT_NAME)

	return &CatalogsReadTestApplicationBuilder{
		ApplicationBuilder: test.NewTestApplicationBuilder(tb),
		tb:                 tb,
	}
}

func (a *CatalogsReadTestApplicationBuilder) Build() *CatalogsReadTestApplication {
	return NewCatalogsReadTestApplication(
		a.tb,
		a.GetProvides(),
		a.GetDecorates(),
		a.Options(),
		a.Logger(),
		a.Environment(),
	)
}
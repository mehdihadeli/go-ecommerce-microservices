//go:build integration
// +build integration

package commands

import (
	"context"
	"testing"

	uuid "github.com/satori/go.uuid"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/mehdihadeli/go-mediatr"

	"github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogreadservice/internal/products/features/deleting_products/v1/commands"
	"github.com/mehdihadeli/go-ecommerce-microservices/internal/services/catalogreadservice/internal/shared/test_fixture/integration"
)

func TestDeleteProduct(t *testing.T) {
	integrationTestSharedFixture := integration.NewIntegrationTestSharedFixture(t)

	Convey("Deleting Product Feature", t, func() {
		ctx := context.Background()
		integrationTestSharedFixture.InitializeTest()

		// https://specflow.org/learn/gherkin/#learn-gherkin
		// scenario
		Convey("Deleting an existing product from the database", func() {
			Convey("Given an existing product in the system", func() {
				productId, err := uuid.FromString(integrationTestSharedFixture.Items[0].ProductId)
				So(err, ShouldBeNil)

				command, err := commands.NewDeleteProduct(productId)
				So(err, ShouldBeNil)

				Convey("When we execute the DeleteProduct command", func() {
					result, err := mediatr.Send[*commands.DeleteProduct, *mediatr.Unit](
						context.Background(),
						command,
					)

					Convey("Then the product should be deleted successfully", func() {
						So(err, ShouldBeNil)
						So(result, ShouldNotBeNil)

						Convey("And the product should no longer exist in the system", func() {
							deletedProduct, _ := integrationTestSharedFixture.ProductRepository.GetProductById(
								ctx,
								productId.String(),
							)
							So(deletedProduct, ShouldBeNil)
						})
					})
				})
			})
		})

		integrationTestSharedFixture.DisposeTest()
	})
}
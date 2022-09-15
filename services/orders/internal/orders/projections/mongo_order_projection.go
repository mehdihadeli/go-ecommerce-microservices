package projections

import (
	"context"
	"emperror.dev/errors"
	"github.com/mehdihadeli/store-golang-microservice-sample/pkg/es/contracts/projection"
	"github.com/mehdihadeli/store-golang-microservice-sample/pkg/es/models"
	kafkaClient "github.com/mehdihadeli/store-golang-microservice-sample/pkg/kafka"
	"github.com/mehdihadeli/store-golang-microservice-sample/pkg/mapper"
	"github.com/mehdihadeli/store-golang-microservice-sample/services/orders/internal/orders/contracts/repositories"
	creatingOrderEvents "github.com/mehdihadeli/store-golang-microservice-sample/services/orders/internal/orders/features/creating_order/events/domain/v1"
	"github.com/mehdihadeli/store-golang-microservice-sample/services/orders/internal/orders/models/orders/read_models"
	uuid "github.com/satori/go.uuid"
)

type mongoOrderProjection struct {
	mongoOrderRepository repositories.OrderReadRepository
	kafkaProducer        kafkaClient.Producer
}

func NewMongoOrderProjection(mongoOrderRepository repositories.OrderReadRepository, kafkaProducer kafkaClient.Producer) projection.IProjection {
	return &mongoOrderProjection{mongoOrderRepository: mongoOrderRepository, kafkaProducer: kafkaProducer}
}

func (m mongoOrderProjection) ProcessEvent(ctx context.Context, streamEvent *models.StreamEvent) error {
	// Handling and projecting event to elastic read model
	switch evt := streamEvent.Event.(type) {

	case *creatingOrderEvents.OrderCreatedEventV1:
		return m.onOrderCreated(ctx, evt)
	}

	return nil
}

func (m *mongoOrderProjection) onOrderCreated(ctx context.Context, evt *creatingOrderEvents.OrderCreatedEventV1) error {
	items, err := mapper.Map[[]*read_models.ShopItemReadModel](evt.ShopItems)
	if err != nil {
		return errors.WrapIf(err, "[mongoOrderProjection_onOrderCreated.Map] error in mapping shopItems")
	}

	orderRead := read_models.NewOrderReadModel(uuid.NewV4(), evt.OrderId, items, evt.AccountEmail, evt.DeliveryAddress, evt.DeliveredTime)
	_, err = m.mongoOrderRepository.CreateOrder(ctx, orderRead)
	if err != nil {
		return errors.WrapIf(err, "[mongoOrderProjection_onOrderCreated.CreateOrder] error in creating order with mongoOrderRepository")
	}

	// TODO: publish integration event

	return nil
}
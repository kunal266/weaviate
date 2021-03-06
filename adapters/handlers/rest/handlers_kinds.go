//                           _       _
// __      _____  __ ___   ___  __ _| |_ ___
// \ \ /\ / / _ \/ _` \ \ / / |/ _` | __/ _ \
//  \ V  V /  __/ (_| |\ V /| | (_| | ||  __/
//   \_/\_/ \___|\__,_| \_/ |_|\__,_|\__\___|
//
//  Copyright © 2016 - 2020 SeMI Technologies B.V. All rights reserved.
//
//  CONTACT: hello@semi.technology
//

package rest

import (
	"context"
	"fmt"
	"strings"

	middleware "github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/semi-technologies/weaviate/adapters/handlers/rest/operations"
	"github.com/semi-technologies/weaviate/adapters/handlers/rest/operations/actions"
	"github.com/semi-technologies/weaviate/adapters/handlers/rest/operations/things"
	"github.com/semi-technologies/weaviate/deprecations"
	"github.com/semi-technologies/weaviate/entities/models"
	"github.com/semi-technologies/weaviate/entities/schema/crossref"
	"github.com/semi-technologies/weaviate/usecases/auth/authorization/errors"
	"github.com/semi-technologies/weaviate/usecases/config"
	"github.com/semi-technologies/weaviate/usecases/kinds"
	"github.com/semi-technologies/weaviate/usecases/projector"
	"github.com/semi-technologies/weaviate/usecases/traverser"
	"github.com/sirupsen/logrus"
)

type kindHandlers struct {
	manager kindsManager
	logger  logrus.FieldLogger
	config  config.Config
}

type requestLog interface {
	Register(string, string)
}

type kindsManager interface {
	AddThing(context.Context, *models.Principal, *models.Thing) (*models.Thing, error)
	AddAction(context.Context, *models.Principal, *models.Action) (*models.Action, error)
	ValidateThing(context.Context, *models.Principal, *models.Thing) error
	ValidateAction(context.Context, *models.Principal, *models.Action) error
	GetThing(context.Context, *models.Principal, strfmt.UUID, traverser.UnderscoreProperties) (*models.Thing, error)
	GetAction(context.Context, *models.Principal, strfmt.UUID, traverser.UnderscoreProperties) (*models.Action, error)
	GetThings(context.Context, *models.Principal, *int64, traverser.UnderscoreProperties) ([]*models.Thing, error)
	GetActions(context.Context, *models.Principal, *int64, traverser.UnderscoreProperties) ([]*models.Action, error)
	UpdateThing(context.Context, *models.Principal, strfmt.UUID, *models.Thing) (*models.Thing, error)
	UpdateAction(context.Context, *models.Principal, strfmt.UUID, *models.Action) (*models.Action, error)
	MergeThing(context.Context, *models.Principal, strfmt.UUID, *models.Thing) error
	MergeAction(context.Context, *models.Principal, strfmt.UUID, *models.Action) error
	DeleteThing(context.Context, *models.Principal, strfmt.UUID) error
	DeleteAction(context.Context, *models.Principal, strfmt.UUID) error
	AddThingReference(context.Context, *models.Principal, strfmt.UUID, string, *models.SingleRef) error
	AddActionReference(context.Context, *models.Principal, strfmt.UUID, string, *models.SingleRef) error
	UpdateThingReferences(context.Context, *models.Principal, strfmt.UUID, string, models.MultipleRef) error
	UpdateActionReferences(context.Context, *models.Principal, strfmt.UUID, string, models.MultipleRef) error
	DeleteThingReference(context.Context, *models.Principal, strfmt.UUID, string, *models.SingleRef) error
	DeleteActionReference(context.Context, *models.Principal, strfmt.UUID, string, *models.SingleRef) error
}

func (h *kindHandlers) addThing(params things.ThingsCreateParams,
	principal *models.Principal) middleware.Responder {
	thing, err := h.manager.AddThing(params.HTTPRequest.Context(), principal, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return things.NewThingsCreateForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrInvalidUserInput:
			return things.NewThingsCreateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return things.NewThingsCreateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	schemaMap, ok := thing.Schema.(map[string]interface{})
	if ok {
		thing.Schema = h.extendSchemaWithAPILinks(schemaMap)
	}

	return things.NewThingsCreateOK().WithPayload(thing)
}

func (h *kindHandlers) validateThing(params things.ThingsValidateParams,
	principal *models.Principal) middleware.Responder {

	err := h.manager.ValidateThing(params.HTTPRequest.Context(), principal, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return things.NewThingsValidateForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrInvalidUserInput:
			return things.NewThingsValidateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return things.NewThingsValidateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return things.NewThingsValidateOK()
}

func (h *kindHandlers) addAction(params actions.ActionsCreateParams,
	principal *models.Principal) middleware.Responder {
	action, err := h.manager.AddAction(params.HTTPRequest.Context(), principal, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return actions.NewActionsCreateForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrInvalidUserInput:
			return actions.NewActionsCreateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return actions.NewActionsCreateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	schemaMap, ok := action.Schema.(map[string]interface{})
	if ok {
		action.Schema = h.extendSchemaWithAPILinks(schemaMap)
	}

	return actions.NewActionsCreateOK().WithPayload(action)
}

func (h *kindHandlers) validateAction(params actions.ActionsValidateParams,
	principal *models.Principal) middleware.Responder {

	err := h.manager.ValidateAction(params.HTTPRequest.Context(), principal, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return actions.NewActionsValidateForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrInvalidUserInput:
			return actions.NewActionsValidateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return actions.NewActionsValidateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return actions.NewActionsValidateOK()
}

func (h *kindHandlers) getThing(params things.ThingsGetParams,
	principal *models.Principal) middleware.Responder {

	underscores, err := parseIncludeParam(params.Include)
	if err != nil {
		return things.NewThingsGetBadRequest().
			WithPayload(errPayloadFromSingleErr(err))
	}

	if derefBool(params.Meta) {
		deprecations.Log(h.logger, "rest-meta-prop")
		underscores.Classification = true
		underscores.RefMeta = true
		underscores.Vector = true
	}

	thing, err := h.manager.GetThing(params.HTTPRequest.Context(), principal, params.ID, underscores)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return things.NewThingsGetForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrNotFound:
			return things.NewThingsGetNotFound()
		default:
			return things.NewThingsGetInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	schemaMap, ok := thing.Schema.(map[string]interface{})
	if ok {
		thing.Schema = h.extendSchemaWithAPILinks(schemaMap)
	}

	return things.NewThingsGetOK().WithPayload(thing)
}

func (h *kindHandlers) getAction(params actions.ActionsGetParams,
	principal *models.Principal) middleware.Responder {
	underscores, err := parseIncludeParam(params.Include)
	if err != nil {
		return actions.NewActionsGetBadRequest().
			WithPayload(errPayloadFromSingleErr(err))
	}

	if derefBool(params.Meta) {
		deprecations.Log(h.logger, "rest-meta-prop")
		underscores.Classification = true
		underscores.RefMeta = true
		underscores.Vector = true
	}
	action, err := h.manager.GetAction(params.HTTPRequest.Context(), principal, params.ID, underscores)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return actions.NewActionsGetForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrNotFound:
			return actions.NewActionsGetNotFound()
		default:
			return actions.NewActionsGetInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	schemaMap, ok := action.Schema.(map[string]interface{})
	if ok {
		action.Schema = h.extendSchemaWithAPILinks(schemaMap)
	}

	return actions.NewActionsGetOK().WithPayload(action)
}

func (h *kindHandlers) getThings(params things.ThingsListParams,
	principal *models.Principal) middleware.Responder {
	underscores, err := parseIncludeParam(params.Include)
	if err != nil {
		return things.NewThingsListBadRequest().
			WithPayload(errPayloadFromSingleErr(err))
	}

	var deprecationsRes []*models.Deprecation

	if derefBool(params.Meta) {
		deprecations.Log(h.logger, "rest-meta-prop")
		d := deprecations.ByID["rest-meta-prop"]
		deprecationsRes = append(deprecationsRes, &d)
		underscores.Classification = true
		underscores.RefMeta = true
		underscores.Vector = true
	}

	list, err := h.manager.GetThings(params.HTTPRequest.Context(), principal, params.Limit, underscores)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return things.NewThingsListForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return things.NewThingsListInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	for i, thing := range list {
		schemaMap, ok := thing.Schema.(map[string]interface{})
		if ok {
			list[i].Schema = h.extendSchemaWithAPILinks(schemaMap)
		}
	}

	return things.NewThingsListOK().
		WithPayload(&models.ThingsListResponse{
			Things:       list,
			TotalResults: int64(len(list)),
			Deprecations: deprecationsRes,
		})
}

func (h *kindHandlers) getActions(params actions.ActionsListParams,
	principal *models.Principal) middleware.Responder {
	underscores, err := parseIncludeParam(params.Include)
	if err != nil {
		return actions.NewActionsListBadRequest().
			WithPayload(errPayloadFromSingleErr(err))
	}

	var deprecationsRes []*models.Deprecation

	if derefBool(params.Meta) {
		deprecations.Log(h.logger, "rest-meta-prop")
		d := deprecations.ByID["rest-meta-prop"]
		deprecationsRes = append(deprecationsRes, &d)
		underscores.Classification = true
		underscores.RefMeta = true
		underscores.Vector = true
	}
	list, err := h.manager.GetActions(params.HTTPRequest.Context(), principal, params.Limit, underscores)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return actions.NewActionsListForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return actions.NewActionsListInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	for i, action := range list {
		schemaMap, ok := action.Schema.(map[string]interface{})
		if ok {
			list[i].Schema = h.extendSchemaWithAPILinks(schemaMap)
		}
	}

	return actions.NewActionsListOK().
		WithPayload(&models.ActionsListResponse{
			Actions:      list,
			Deprecations: deprecationsRes,
			TotalResults: int64(len(list)),
		})
}

func (h *kindHandlers) updateThing(params things.ThingsUpdateParams,
	principal *models.Principal) middleware.Responder {
	thing, err := h.manager.UpdateThing(params.HTTPRequest.Context(), principal, params.ID, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return things.NewThingsUpdateForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrInvalidUserInput:
			return things.NewThingsUpdateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return things.NewThingsUpdateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	schemaMap, ok := thing.Schema.(map[string]interface{})
	if ok {
		thing.Schema = h.extendSchemaWithAPILinks(schemaMap)
	}

	return things.NewThingsUpdateOK().WithPayload(thing)
}

func (h *kindHandlers) updateAction(params actions.ActionsUpdateParams,
	principal *models.Principal) middleware.Responder {
	action, err := h.manager.UpdateAction(params.HTTPRequest.Context(), principal, params.ID, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return actions.NewActionsUpdateForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrInvalidUserInput:
			return actions.NewActionsUpdateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return actions.NewActionsUpdateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	schemaMap, ok := action.Schema.(map[string]interface{})
	if ok {
		action.Schema = h.extendSchemaWithAPILinks(schemaMap)
	}

	return actions.NewActionsUpdateOK().WithPayload(action)
}

func (h *kindHandlers) deleteThing(params things.ThingsDeleteParams,
	principal *models.Principal) middleware.Responder {
	err := h.manager.DeleteThing(params.HTTPRequest.Context(), principal, params.ID)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return things.NewThingsDeleteForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrNotFound:
			return things.NewThingsDeleteNotFound()
		default:
			return things.NewThingsDeleteInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return things.NewThingsDeleteNoContent()
}

func (h *kindHandlers) deleteAction(params actions.ActionsDeleteParams,
	principal *models.Principal) middleware.Responder {
	err := h.manager.DeleteAction(params.HTTPRequest.Context(), principal, params.ID)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return actions.NewActionsDeleteForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrNotFound:
			return actions.NewActionsDeleteNotFound()
		default:
			return actions.NewActionsDeleteInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return actions.NewActionsDeleteNoContent()
}

func (h *kindHandlers) patchThing(params things.ThingsPatchParams, principal *models.Principal) middleware.Responder {

	err := h.manager.MergeThing(params.HTTPRequest.Context(), principal, params.ID, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return things.NewThingsPatchForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrInvalidUserInput:
			return things.NewThingsUpdateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return things.NewThingsUpdateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return things.NewThingsPatchNoContent()
}

func (h *kindHandlers) patchAction(params actions.ActionsPatchParams, principal *models.Principal) middleware.Responder {
	err := h.manager.MergeAction(params.HTTPRequest.Context(), principal, params.ID, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return actions.NewActionsPatchForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrInvalidUserInput:
			return actions.NewActionsUpdateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return actions.NewActionsUpdateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return actions.NewActionsPatchNoContent()
}

func (h *kindHandlers) addThingReference(params things.ThingsReferencesCreateParams,
	principal *models.Principal) middleware.Responder {
	err := h.manager.AddThingReference(params.HTTPRequest.Context(), principal, params.ID, params.PropertyName, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return things.NewThingsReferencesCreateForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrNotFound, kinds.ErrInvalidUserInput:
			return things.NewThingsReferencesCreateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return things.NewThingsReferencesCreateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return things.NewThingsReferencesCreateOK()
}

func (h *kindHandlers) addActionReference(params actions.ActionsReferencesCreateParams,
	principal *models.Principal) middleware.Responder {
	err := h.manager.AddActionReference(params.HTTPRequest.Context(), principal, params.ID, params.PropertyName, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return actions.NewActionsReferencesCreateForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrNotFound, kinds.ErrInvalidUserInput:
			return actions.NewActionsReferencesCreateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return actions.NewActionsReferencesCreateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return actions.NewActionsReferencesCreateOK()
}

func (h *kindHandlers) updateActionReferences(params actions.ActionsReferencesUpdateParams,
	principal *models.Principal) middleware.Responder {
	err := h.manager.UpdateActionReferences(params.HTTPRequest.Context(), principal, params.ID, params.PropertyName, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return actions.NewActionsReferencesUpdateForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrNotFound, kinds.ErrInvalidUserInput:
			return actions.NewActionsReferencesUpdateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return actions.NewActionsReferencesUpdateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return actions.NewActionsReferencesUpdateOK()
}

func (h *kindHandlers) updateThingReferences(params things.ThingsReferencesUpdateParams,
	principal *models.Principal) middleware.Responder {
	err := h.manager.UpdateThingReferences(params.HTTPRequest.Context(), principal, params.ID, params.PropertyName, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return things.NewThingsReferencesUpdateForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrNotFound, kinds.ErrInvalidUserInput:
			return things.NewThingsReferencesUpdateUnprocessableEntity().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return things.NewThingsReferencesUpdateInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return things.NewThingsReferencesUpdateOK()
}

func (h *kindHandlers) deleteActionReference(params actions.ActionsReferencesDeleteParams,
	principal *models.Principal) middleware.Responder {
	err := h.manager.DeleteActionReference(params.HTTPRequest.Context(), principal, params.ID, params.PropertyName, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return actions.NewActionsReferencesDeleteForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrNotFound, kinds.ErrInvalidUserInput:
			return actions.NewActionsReferencesDeleteNotFound().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return actions.NewActionsReferencesDeleteInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return actions.NewActionsReferencesDeleteNoContent()
}

func (h *kindHandlers) deleteThingReference(params things.ThingsReferencesDeleteParams,
	principal *models.Principal) middleware.Responder {
	err := h.manager.DeleteThingReference(params.HTTPRequest.Context(), principal, params.ID, params.PropertyName, params.Body)
	if err != nil {
		switch err.(type) {
		case errors.Forbidden:
			return things.NewThingsReferencesDeleteForbidden().
				WithPayload(errPayloadFromSingleErr(err))
		case kinds.ErrNotFound, kinds.ErrInvalidUserInput:
			return things.NewThingsReferencesDeleteNotFound().
				WithPayload(errPayloadFromSingleErr(err))
		default:
			return things.NewThingsReferencesDeleteInternalServerError().
				WithPayload(errPayloadFromSingleErr(err))
		}
	}

	return things.NewThingsReferencesDeleteNoContent()
}

func setupKindHandlers(api *operations.WeaviateAPI,
	manager *kinds.Manager, config config.Config, logger logrus.FieldLogger) {
	h := &kindHandlers{manager, logger, config}

	api.ThingsThingsCreateHandler = things.
		ThingsCreateHandlerFunc(h.addThing)
	api.ThingsThingsValidateHandler = things.
		ThingsValidateHandlerFunc(h.validateThing)
	api.ThingsThingsGetHandler = things.
		ThingsGetHandlerFunc(h.getThing)
	api.ThingsThingsDeleteHandler = things.
		ThingsDeleteHandlerFunc(h.deleteThing)
	api.ThingsThingsListHandler = things.
		ThingsListHandlerFunc(h.getThings)
	api.ThingsThingsUpdateHandler = things.
		ThingsUpdateHandlerFunc(h.updateThing)
	api.ThingsThingsPatchHandler = things.
		ThingsPatchHandlerFunc(h.patchThing)
	api.ThingsThingsReferencesCreateHandler = things.
		ThingsReferencesCreateHandlerFunc(h.addThingReference)
	api.ThingsThingsReferencesDeleteHandler = things.
		ThingsReferencesDeleteHandlerFunc(h.deleteThingReference)
	api.ThingsThingsReferencesUpdateHandler = things.
		ThingsReferencesUpdateHandlerFunc(h.updateThingReferences)

	api.ActionsActionsCreateHandler = actions.
		ActionsCreateHandlerFunc(h.addAction)
	api.ActionsActionsValidateHandler = actions.
		ActionsValidateHandlerFunc(h.validateAction)
	api.ActionsActionsGetHandler = actions.
		ActionsGetHandlerFunc(h.getAction)
	api.ActionsActionsDeleteHandler = actions.
		ActionsDeleteHandlerFunc(h.deleteAction)
	api.ActionsActionsListHandler = actions.
		ActionsListHandlerFunc(h.getActions)
	api.ActionsActionsUpdateHandler = actions.
		ActionsUpdateHandlerFunc(h.updateAction)
	api.ActionsActionsPatchHandler = actions.
		ActionsPatchHandlerFunc(h.patchAction)
	api.ActionsActionsReferencesCreateHandler = actions.
		ActionsReferencesCreateHandlerFunc(h.addActionReference)
	api.ActionsActionsReferencesDeleteHandler = actions.
		ActionsReferencesDeleteHandlerFunc(h.deleteActionReference)
	api.ActionsActionsReferencesUpdateHandler = actions.
		ActionsReferencesUpdateHandlerFunc(h.updateActionReferences)

}

func derefBool(in *bool) bool {
	if in == nil {
		return false
	}

	return *in
}

func (h *kindHandlers) extendSchemaWithAPILinks(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return schema
	}

	for key, value := range schema {
		asMultiRef, ok := value.(models.MultipleRef)
		if !ok {
			continue
		}

		schema[key] = h.extendReferencesWithAPILinks(asMultiRef)
	}
	return schema
}

func (h *kindHandlers) extendReferencesWithAPILinks(refs models.MultipleRef) models.MultipleRef {
	for i, ref := range refs {
		refs[i] = h.extendReferenceWithAPILink(ref)
	}

	return refs
}

func (h *kindHandlers) extendReferenceWithAPILink(ref *models.SingleRef) *models.SingleRef {

	parsed, err := crossref.Parse(ref.Beacon.String())
	if err != nil {
		// ignore return unchanged
		return ref
	}

	ref.Href = strfmt.URI(fmt.Sprintf("%s/v1/%ss/%s", h.config.Origin, parsed.Kind.Name(), parsed.TargetID))
	return ref
}

func parseIncludeParam(in *string) (traverser.UnderscoreProperties, error) {
	out := traverser.UnderscoreProperties{}
	if in == nil {
		return out, nil
	}

	parts := strings.Split(*in, ",")

	for _, prop := range parts {
		switch prop {
		case "_classification", "classification":
			out.Classification = true
			out.RefMeta = true
		case "_interpretation", "interpretation":
			out.Interpretation = true
		case "_nearestNeighbors", "nearestNeighbors", "nearestneighbors", "_nearestneighbors", "nearest-neighbors", "nearest_neighbors", "_nearest_neighbors":
			out.NearestNeighbors = true
		case "_featureProjection", "featureProjection", "featureprojection", "_featureprojection", "feature-projection", "feature_projection", "_feature_projection":
			out.FeatureProjection = &projector.Params{}
		case "_vector", "vector":
			out.Vector = true

		default:
			return out, fmt.Errorf("unrecognized property '%s' in ?include list", prop)
		}
	}

	return out, nil
}

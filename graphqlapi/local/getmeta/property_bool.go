package getmeta

import (
	"fmt"

	"github.com/creativesoftwarefdn/weaviate/graphqlapi/descriptions"
	"github.com/creativesoftwarefdn/weaviate/models"
	"github.com/graphql-go/graphql"
)

func booleanPropertyFields(class *models.SemanticSchemaClass,
	property *models.SemanticSchemaClassProperty) *graphql.Object {
	getMetaBooleanFields := graphql.Fields{
		"type": &graphql.Field{
			Name:        fmt.Sprintf("Meta%s%sType", class.Class, property.Name),
			Description: descriptions.GetMetaPropertyTypeDesc,
			Type:        graphql.String,
		},
		"count": &graphql.Field{
			Name:        fmt.Sprintf("Meta%s%sCount", class.Class, property.Name),
			Description: descriptions.GetMetaPropertyCountDesc,
			Type:        graphql.Int,
		},
		"totalTrue": &graphql.Field{
			Name:        fmt.Sprintf("Meta%s%sTotalTrue", class.Class, property.Name),
			Description: descriptions.GetMetaClassPropertyTotalTrueDesc,
			Type:        graphql.Int,
		},
		"percentageTrue": &graphql.Field{
			Name:        fmt.Sprintf("Meta%s%sPercentageTrue", class.Class, property.Name),
			Description: descriptions.GetMetaClassPropertyPercentageTrueDesc,
			Type:        graphql.Float,
		},
		"totalFalse": &graphql.Field{
			Name:        fmt.Sprintf("Meta%s%sTotalFalse", class.Class, property.Name),
			Description: descriptions.GetMetaClassPropertyTotalFalseDesc,
			Type:        graphql.Int,
		},
		"percentageFalse": &graphql.Field{
			Name:        fmt.Sprintf("Meta%s%sPercentageFalse", class.Class, property.Name),
			Description: descriptions.GetMetaClassPropertyPercentageFalseDesc,
			Type:        graphql.Float,
		},
	}

	getMetaBooleanProperty := graphql.ObjectConfig{
		Name:        fmt.Sprintf("Meta%s%sObj", class.Class, property.Name),
		Fields:      getMetaBooleanFields,
		Description: descriptions.GetMetaPropertyObjectDesc,
	}

	return graphql.NewObject(getMetaBooleanProperty)
}
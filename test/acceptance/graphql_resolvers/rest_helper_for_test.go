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

package test

import (
	"testing"

	"github.com/go-openapi/strfmt"
	"github.com/semi-technologies/weaviate/client/things"
	"github.com/semi-technologies/weaviate/entities/models"
	"github.com/semi-technologies/weaviate/test/acceptance/helper"
)

func assertGetThing(t *testing.T, uuid strfmt.UUID) *models.Thing {
	getResp, err := helper.Client(t).Things.ThingsGet(things.NewThingsGetParams().WithID(uuid), nil)

	var thing *models.Thing

	helper.AssertRequestOk(t, getResp, err, func() {
		thing = getResp.Payload
	})

	return thing
}

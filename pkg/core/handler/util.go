// Copyright The Karbour Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/render"
)

func HandleResult(w http.ResponseWriter, r *http.Request, ctx context.Context, err error, data any) {
	if err != nil {
		render.Render(w, r, FailureResponse(ctx, err))
		return
	}
	render.JSON(w, r, SuccessResponse(ctx, data))
}
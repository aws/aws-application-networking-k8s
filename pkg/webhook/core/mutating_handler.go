package core

import (
	"context"
	"encoding/json"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	admissionv1 "k8s.io/api/admission/v1"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type mutatingHandler struct {
	log     gwlog.Logger
	mutator Mutator
	decoder *admission.Decoder
}

func (h *mutatingHandler) SetDecoder(d *admission.Decoder) {
	h.decoder = d
}

// Handle handles admission requests.
func (h *mutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	h.log.Debugw(ctx, "mutating webhook request", "operation", req.Operation, "name", req.Name, "namespace", req.Namespace)
	var resp admission.Response
	switch req.Operation {
	case admissionv1.Create:
		resp = h.handleCreate(ctx, req)
	case admissionv1.Update:
		resp = h.handleUpdate(ctx, req)
	default:
		resp = admission.Allowed("")
	}
	h.log.Debugw(ctx, "mutating webhook response", "patches", resp.Patches)
	return resp
}

func (h *mutatingHandler) handleCreate(ctx context.Context, req admission.Request) admission.Response {
	prototype, err := h.mutator.Prototype(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	obj := prototype.DeepCopyObject()
	if err := h.decoder.DecodeRaw(req.Object, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	mutatedObj, err := h.mutator.MutateCreate(ContextWithAdmissionRequest(ctx, req), obj)
	if err != nil {
		return admission.Denied(err.Error())
	}
	mutatedObjPayload, err := json.Marshal(mutatedObj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, mutatedObjPayload)
}

func (h *mutatingHandler) handleUpdate(ctx context.Context, req admission.Request) admission.Response {
	prototype, err := h.mutator.Prototype(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	obj := prototype.DeepCopyObject()
	oldObj := prototype.DeepCopyObject()
	if err := h.decoder.DecodeRaw(req.Object, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err := h.decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	mutatedObj, err := h.mutator.MutateUpdate(ContextWithAdmissionRequest(ctx, req), obj, oldObj)
	if err != nil {
		return admission.Denied(err.Error())
	}
	mutatedObjPayload, err := json.Marshal(mutatedObj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, mutatedObjPayload)
}

package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	webhook "github.com/sonymuhamad/webhook-lab"
)

const maxRequestBodyBytes = 1 << 20

var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	// Report fields by their JSON name, which is what the client sent.
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		return name
	})
	return v
}

// decodeJSON decodes the body into dst and validates its `validate` tags.
// Both kinds of failure come back as webhook.ValidationError.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return webhook.ValidationError{Message: fmt.Sprintf("invalid JSON body: %v", err)}
	}

	err := validate.Struct(dst)
	var fieldErrs validator.ValidationErrors
	if errors.As(err, &fieldErrs) {
		messages := make([]string, len(fieldErrs))
		for i, fe := range fieldErrs {
			messages[i] = fieldErrorMessage(fe)
		}
		return webhook.ValidationError{Message: strings.Join(messages, "; ")}
	}
	return err
}

func fieldErrorMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fe.Field() + " is required"
	case "max":
		return fmt.Sprintf("%s must be at most %s characters", fe.Field(), fe.Param())
	case "http_url":
		return fe.Field() + " must be an absolute http or https URL"
	default:
		return fmt.Sprintf("%s failed %q validation", fe.Field(), fe.Tag())
	}
}

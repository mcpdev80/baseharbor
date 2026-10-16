package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// appFootprintShowReader is the existing read-only GuidedReadContext authority.
// Command registration and argument routing belong to core-921.
type appFootprintShowReader func(context.Context) (GuidedReadContext, error)

type appFootprintShowOptions struct {
	JSON    bool
	Details bool
	Target  string
}

// appFootprintShow writes the same authoritative Footprint in either format.
// It never calculates a footprint, queries a runtime or persists state.
func appFootprintShow(ctx context.Context, out io.Writer, read appFootprintShowReader, options appFootprintShowOptions) error {
	if read == nil || out == nil || options.Target == "" {
		return errors.New("footprint: missing target or read interface")
	}
	view, err := read(ctx)
	if err != nil {
		var readErr *GuidedReadError
		if errors.As(err, &readErr) {
			switch readErr.Kind {
			case "wrong_target", "missing", "denied", "unverifiable", "source_missing", "incompatible", "ambiguous":
				return fmt.Errorf("footprint: %s", readErr.Kind)
			}
		}
		return errors.New("footprint: unavailable")
	}
	return appFootprintShowContext(out, view, options)
}

// appFootprintShowContext is callable from CLI and machine adapters without
// registering a new command, exposing revisions or re-running resolution.
func appFootprintShowContext(out io.Writer, view GuidedReadContext, options appFootprintShowOptions) error {
	if out == nil || options.Target == "" || view.Target == "" ||
		view.Footprint.Target == "" || options.Target != view.Target ||
		options.Target != view.Footprint.Target {
		return errors.New("footprint: wrong_target")
	}
	if view.Footprint.Version != application.FootprintVersion {
		return errors.New("footprint: incompatible")
	}
	if !options.JSON {
		_, err := io.WriteString(out, presentAppFootprint(view, appFootprintPresenterOptions{Details: options.Details})+"\n")
		return err
	}
	// Preserve the canonical domain JSON schema; do not serialize the wider
	// GuidedReadContext (which also carries revisions and other private data).
	payload, err := json.Marshal(view.Footprint)
	if err != nil {
		return errors.New("footprint: unavailable")
	}
	_, err = out.Write(append(payload, '\n'))
	return err
}

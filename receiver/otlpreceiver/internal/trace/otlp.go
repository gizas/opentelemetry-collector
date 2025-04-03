// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package trace // import "go.opentelemetry.io/collector/receiver/otlpreceiver/internal/trace"

import (
	"context"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"go.opentelemetry.io/collector/receiver/receiverhelper"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const dataFormatProtobuf = "protobuf"

// Receiver is the type used to handle traces from OpenTelemetry exporters.
type Receiver struct {
	ptraceotlp.UnimplementedGRPCServer
	nextConsumer consumer.Traces
	obsreport    *receiverhelper.ObsReport
}

// New creates a new Receiver reference.
func New(nextConsumer consumer.Traces, obsreport *receiverhelper.ObsReport) *Receiver {
	return &Receiver{
		nextConsumer: nextConsumer,
		obsreport:    obsreport,
	}
}

// Export implements the OTLP traces receiver protocol.
func (r *Receiver) Export(ctx context.Context, req ptraceotlp.ExportRequest) (ptraceotlp.ExportResponse, error) {
	td := req.Traces()
	spanCount := td.SpanCount()
	if spanCount == 0 {
		return ptraceotlp.NewExportResponse(), nil
	}

	ctx = r.obsreport.StartTracesOp(ctx)
	err := r.nextConsumer.ConsumeTraces(ctx, td)
	r.obsreport.EndTracesOp(ctx, dataFormatProtobuf, spanCount, err)

	// Use appropriate status codes for permanent/non-permanent errors
	// If we return the error straightaway, then the grpc implementation will set status code to Unknown
	// Refer: https://github.com/grpc/grpc-go/blob/v1.59.0/server.go#L1345
	// So, convert the error to appropriate grpc status and return the error
	// NonPermanent errors will be converted to codes.Unavailable (equivalent to HTTP 503)
	// Permanent errors will be converted to codes.InvalidArgument (equivalent to HTTP 400)
	if err != nil {
		s, ok := status.FromError(err)
		if !ok {
			// Default to a retryable error
			// https://github.com/open-telemetry/opentelemetry-proto/blob/main/docs/specification.md#failures
			code := codes.Unavailable
			if consumererror.IsPermanent(err) {
				// If an error is permanent but doesn't have an attached gRPC status, assume it is server-side.
				code = codes.Internal
			}
			s = status.New(code, err.Error())
		}
		return ptraceotlp.NewExportResponse(), s.Err()
	}

	return ptraceotlp.NewExportResponse(), nil
}

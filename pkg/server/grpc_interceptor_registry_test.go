package server

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

func TestRegisterPreAuthGRPCUnaryInterceptor(t *testing.T) {
	// Reset global state for clean testing
	originalUnary := preAuthUnaryInterceptors
	defer func() {
		preAuthUnaryInterceptors = originalUnary
	}()
	preAuthUnaryInterceptors = nil

	// Test registering unary interceptors
	called := []string{}

	interceptor1 := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		called = append(called, "interceptor1")
		return handler(ctx, req)
	}

	interceptor2 := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		called = append(called, "interceptor2")
		return handler(ctx, req)
	}

	// Register interceptors
	RegisterPreAuthGRPCUnaryInterceptor(interceptor1)
	RegisterPreAuthGRPCUnaryInterceptor(interceptor2)

	// Verify they were registered in order
	if len(preAuthUnaryInterceptors) != 2 {
		t.Errorf("Expected 2 registered interceptors, got %d", len(preAuthUnaryInterceptors))
	}

	// Test that interceptors are called in registration order
	finalHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		called = append(called, "final")
		return "result", nil
	}

	// Simulate interceptor chain execution
	ctx := context.Background()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	var handler grpc.UnaryHandler = finalHandler
	// Execute in reverse order (as gRPC would)
	for i := len(preAuthUnaryInterceptors) - 1; i >= 0; i-- {
		currentHandler := handler
		interceptor := preAuthUnaryInterceptors[i]
		handler = func(ctx context.Context, req interface{}) (interface{}, error) {
			return interceptor(ctx, req, info, currentHandler)
		}
	}

	result, err := handler(ctx, "test-request")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "result" {
		t.Errorf("Expected 'result', got %v", result)
	}

	// Verify call order (should be interceptor1, interceptor2, final)
	expected := []string{"interceptor1", "interceptor2", "final"}
	if len(called) != len(expected) {
		t.Errorf("Expected %d calls, got %d", len(expected), len(called))
	}
	for i, expectedCall := range expected {
		if i >= len(called) || called[i] != expectedCall {
			t.Errorf("Expected call %d to be %s, got %s", i, expectedCall, called[i])
		}
	}
}

func TestRegisterPreAuthGRPCStreamInterceptor(t *testing.T) {
	// Reset global state for clean testing
	originalStream := preAuthStreamInterceptors
	defer func() {
		preAuthStreamInterceptors = originalStream
	}()
	preAuthStreamInterceptors = nil

	// Test registering stream interceptors
	called := []string{}

	interceptor1 := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		called = append(called, "stream1")
		return handler(srv, ss)
	}

	interceptor2 := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		called = append(called, "stream2")
		return handler(srv, ss)
	}

	// Register interceptors
	RegisterPreAuthGRPCStreamInterceptor(interceptor1)
	RegisterPreAuthGRPCStreamInterceptor(interceptor2)

	// Verify they were registered in order
	if len(preAuthStreamInterceptors) != 2 {
		t.Errorf("Expected 2 registered interceptors, got %d", len(preAuthStreamInterceptors))
	}

	// Test that interceptors are called in registration order
	finalHandler := func(srv interface{}, ss grpc.ServerStream) error {
		called = append(called, "stream-final")
		return nil
	}

	// Simulate interceptor chain execution
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/StreamMethod"}

	var handler grpc.StreamHandler = finalHandler
	// Execute in reverse order (as gRPC would)
	for i := len(preAuthStreamInterceptors) - 1; i >= 0; i-- {
		currentHandler := handler
		interceptor := preAuthStreamInterceptors[i]
		handler = func(srv interface{}, ss grpc.ServerStream) error {
			return interceptor(srv, ss, info, currentHandler)
		}
	}

	err := handler("test-service", nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify call order (should be stream1, stream2, stream-final)
	expected := []string{"stream1", "stream2", "stream-final"}
	if len(called) != len(expected) {
		t.Errorf("Expected %d calls, got %d", len(expected), len(called))
	}
	for i, expectedCall := range expected {
		if i >= len(called) || called[i] != expectedCall {
			t.Errorf("Expected call %d to be %s, got %s", i, expectedCall, called[i])
		}
	}
}

func TestGRPCInterceptorRegistryEmptyState(t *testing.T) {
	// Reset global state
	originalUnary := preAuthUnaryInterceptors
	originalStream := preAuthStreamInterceptors
	defer func() {
		preAuthUnaryInterceptors = originalUnary
		preAuthStreamInterceptors = originalStream
	}()
	preAuthUnaryInterceptors = nil
	preAuthStreamInterceptors = nil

	// Verify empty state doesn't break anything
	if len(preAuthUnaryInterceptors) != 0 {
		t.Errorf("Expected empty unary interceptors, got %d", len(preAuthUnaryInterceptors))
	}
	if len(preAuthStreamInterceptors) != 0 {
		t.Errorf("Expected empty stream interceptors, got %d", len(preAuthStreamInterceptors))
	}

	// Registering should work from empty state
	dummyUnary := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	}
	dummyStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, ss)
	}

	RegisterPreAuthGRPCUnaryInterceptor(dummyUnary)
	RegisterPreAuthGRPCStreamInterceptor(dummyStream)

	if len(preAuthUnaryInterceptors) != 1 {
		t.Errorf("Expected 1 unary interceptor after registration, got %d", len(preAuthUnaryInterceptors))
	}
	if len(preAuthStreamInterceptors) != 1 {
		t.Errorf("Expected 1 stream interceptor after registration, got %d", len(preAuthStreamInterceptors))
	}
}

func TestGRPCInterceptorRegistryThreadSafety(t *testing.T) {
	// Reset global state
	originalUnary := preAuthUnaryInterceptors
	originalStream := preAuthStreamInterceptors
	defer func() {
		preAuthUnaryInterceptors = originalUnary
		preAuthStreamInterceptors = originalStream
	}()
	preAuthUnaryInterceptors = nil
	preAuthStreamInterceptors = nil

	// Test concurrent registration (basic test - not comprehensive thread safety)
	done := make(chan bool)

	dummyUnary := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	}
	dummyStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, ss)
	}

	// Register from multiple goroutines
	go func() {
		for i := 0; i < 10; i++ {
			RegisterPreAuthGRPCUnaryInterceptor(dummyUnary)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			RegisterPreAuthGRPCStreamInterceptor(dummyStream)
		}
		done <- true
	}()

	// Wait for completion
	<-done
	<-done

	// Should have registered all interceptors
	if len(preAuthUnaryInterceptors) != 10 {
		t.Errorf("Expected 10 unary interceptors, got %d", len(preAuthUnaryInterceptors))
	}
	if len(preAuthStreamInterceptors) != 10 {
		t.Errorf("Expected 10 stream interceptors, got %d", len(preAuthStreamInterceptors))
	}
}

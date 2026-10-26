package grpc

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Raoon-Tech/bpmn-ai/api/grpc/pb"
	"github.com/Raoon-Tech/bpmn-ai/internal/engine"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/internal/process"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// Server wraps the gRPC server.
type Server struct {
	grpc *grpc.Server
	addr string
}

// NewServer creates a gRPC server bound to the given store/engine.
func NewServer(addr string, svc *Service, logger *observability.Logger) *Server {
	s := grpc.NewServer(
		grpc.UnaryInterceptor(loggingUnaryInterceptor(logger)),
	)
	pb.RegisterHealthServiceServer(s, svc)
	pb.RegisterProcessServiceServer(s, svc)
	pb.RegisterCaseServiceServer(s, svc)
	pb.RegisterTaskServiceServer(s, svc)
	pb.RegisterMessageServiceServer(s, svc)
	pb.RegisterSignalServiceServer(s, svc)
	reflection.Register(s)
	return &Server{grpc: s, addr: addr}
}

// Service implements all gRPC service interfaces.
type Service struct {
	pb.UnimplementedHealthServiceServer
	pb.UnimplementedProcessServiceServer
	pb.UnimplementedCaseServiceServer
	pb.UnimplementedTaskServiceServer
	pb.UnimplementedMessageServiceServer
	pb.UnimplementedSignalServiceServer

	Store  store.Store
	Engine engine.Engine
	Logger *observability.Logger
}

// Start starts listening.
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	return s.grpc.Serve(lis)
}

// Stop gracefully stops the server.
func (s *Server) Stop() { s.grpc.GracefulStop() }

// --- Health ---

func (svc *Service) Check(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{Status: "ok", Version: "dev"}, nil
}

// --- Process ---

func (svc *Service) DeployProcess(ctx context.Context, req *pb.DeployProcessRequest) (*pb.DeployProcessResponse, error) {
	proc, err := bpmn.NewParser().Parse([]byte(req.BpmnXml))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}
	proc.Name = req.Name
	if err := svc.Store.SaveProcess(ctx, proc); err != nil {
		return nil, status.Errorf(codes.Internal, "save: %v", err)
	}
	return &pb.DeployProcessResponse{Id: proc.ID, Name: proc.Name}, nil
}

func (svc *Service) GetProcess(ctx context.Context, req *pb.GetProcessRequest) (*pb.ProcessResponse, error) {
	proc, err := svc.Store.GetProcess(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &pb.ProcessResponse{
		Id:        proc.ID,
		Name:      proc.Name,
		Version:   int32(proc.Version),
		CreatedAt: timestamppb.New(proc.CreatedAt),
	}, nil
}

func (svc *Service) ListProcesses(ctx context.Context, req *pb.ListProcessesRequest) (*pb.ListProcessesResponse, error) {
	procs, err := svc.Store.ListProcesses(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	out := &pb.ListProcessesResponse{}
	for _, p := range procs {
		out.Processes = append(out.Processes, &pb.ProcessResponse{
			Id:        p.ID,
			Name:      p.Name,
			Version:   int32(p.Version),
			CreatedAt: timestamppb.New(p.CreatedAt),
		})
	}
	return out, nil
}

func (svc *Service) DeleteProcess(ctx context.Context, req *pb.DeleteProcessRequest) (*pb.DeleteProcessResponse, error) {
	if err := svc.Store.DeleteProcess(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.DeleteProcessResponse{Deleted: true}, nil
}

// --- Case ---

func (svc *Service) StartCase(ctx context.Context, req *pb.StartCaseRequest) (*pb.StartCaseResponse, error) {
	proc, err := svc.Store.GetProcess(ctx, req.ProcessId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "process: %v", err)
	}
	vars := protoToMap(req.Variables)
	instance := process.NewInstance(proc, vars)
	if req.Title != "" {
		instance.Title = req.Title
	}
	if err := svc.Store.CreateInstance(ctx, instance.ToRecord()); err != nil {
		return nil, status.Errorf(codes.Internal, "create case: %v", err)
	}
	if svc.Engine != nil {
		go func() {
			ectx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = svc.Engine.Run(ectx, instance)
		}()
	}
	return &pb.StartCaseResponse{Id: instance.ID, Status: string(instance.State)}, nil
}

func (svc *Service) GetCase(ctx context.Context, req *pb.GetCaseRequest) (*pb.CaseResponse, error) {
	rec, err := svc.Store.GetInstance(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	resp := &pb.CaseResponse{
		Id:          rec.ID,
		ProcessId:   rec.ProcessID,
		Title:       rec.Title,
		Status:      string(rec.Status),
		CurrentUser: rec.CurrentUser,
		StartedAt:   timestamppb.New(rec.StartedAt),
	}
	if rec.FinishedAt != nil {
		resp.FinishedAt = timestamppb.New(*rec.FinishedAt)
	}
	if rec.Variables != nil {
		resp.Variables = mapToProto(rec.Variables)
	}
	return resp, nil
}

func (svc *Service) ListCases(ctx context.Context, req *pb.ListCasesRequest) (*pb.ListCasesResponse, error) {
	cases, err := svc.Store.ListInstances(ctx, store.InstanceStatus(req.Status))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	out := &pb.ListCasesResponse{}
	for _, c := range cases {
		out.Cases = append(out.Cases, &pb.CaseResponse{
			Id:          c.ID,
			ProcessId:   c.ProcessID,
			Title:       c.Title,
			Status:      string(c.Status),
			CurrentUser: c.CurrentUser,
			StartedAt:   timestamppb.New(c.StartedAt),
		})
	}
	return out, nil
}

func (svc *Service) CancelCase(ctx context.Context, req *pb.CancelCaseRequest) (*pb.CancelCaseResponse, error) {
	rec, err := svc.Store.GetInstance(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	rec.Status = store.InstanceStatusTerminated
	if err := svc.Store.UpdateInstance(ctx, rec); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.CancelCaseResponse{Cancelled: true}, nil
}

func (svc *Service) GetCaseHistory(ctx context.Context, req *pb.GetCaseHistoryRequest) (*pb.GetCaseHistoryResponse, error) {
	logs, err := svc.Store.GetExecutionLog(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	out := &pb.GetCaseHistoryResponse{}
	for _, e := range logs {
		out.Entries = append(out.Entries, &pb.HistoryEntry{
			Id:          e.ID,
			ElementId:   e.ElementID,
			ElementType: string(e.ElementType),
			Action:      e.Action,
			DurationMs:  int32(e.DurationMs),
			CreatedAt:   timestamppb.New(e.CreatedAt),
		})
	}
	return out, nil
}

// --- Task ---

func (svc *Service) ListTasks(ctx context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	flows, err := svc.Store.GetFlowsByInstance(ctx, req.CaseId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	out := &pb.ListTasksResponse{}
	for _, f := range flows {
		if f.ElementType == bpmn.ElementTypeUserTask && f.Status == store.FlowStatusActive {
			out.Tasks = append(out.Tasks, &pb.TaskInfo{
				FlowId:     f.ID,
				ElementId:  f.ElementID,
				Status:     string(f.Status),
				ThreadId:   int32(f.ThreadID),
			})
		}
	}
	return out, nil
}

func (svc *Service) ClaimTask(ctx context.Context, req *pb.ClaimTaskRequest) (*pb.ClaimTaskResponse, error) {
	flow, err := svc.Store.GetFlow(ctx, req.FlowId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	if flow.Status != store.FlowStatusActive {
		return nil, status.Errorf(codes.FailedPrecondition, "task not active")
	}
	if err := svc.Store.UpdateFlow(ctx, flow); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.ClaimTaskResponse{FlowId: flow.ID, ClaimedBy: req.UserId, Status: string(flow.Status)}, nil
}

func (svc *Service) CompleteTask(ctx context.Context, req *pb.CompleteTaskRequest) (*pb.CompleteTaskResponse, error) {
	flow, err := svc.Store.GetFlow(ctx, req.FlowId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	if flow.Status != store.FlowStatusActive {
		return nil, status.Errorf(codes.FailedPrecondition, "task not active")
	}
	now := time.Now()
	flow.Status = store.FlowStatusCompleted
	flow.FinishedAt = &now
	d := int(time.Since(*flow.StartedAt).Milliseconds())
	flow.DurationMs = &d
	if err := svc.Store.UpdateFlow(ctx, flow); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	if svc.Engine != nil {
		go func() {
			ectx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = svc.Engine.Continue(ectx, flow.InstanceID, flow.ID, protoToMap(req.Variables))
		}()
	}
	return &pb.CompleteTaskResponse{TaskId: flow.ID, Status: string(flow.Status)}, nil
}

// --- Message ---

func (svc *Service) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	if svc.Engine == nil {
		return nil, status.Errorf(codes.Unavailable, "engine unavailable")
	}
	if err := svc.Engine.SendMessage(ctx, req.InstanceId, req.MessageRef, protoToMap(req.Variables)); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.SendMessageResponse{Delivered: true}, nil
}

// --- Signal ---

func (svc *Service) SendSignal(ctx context.Context, req *pb.SendSignalRequest) (*pb.SendSignalResponse, error) {
	if svc.Engine == nil {
		return nil, status.Errorf(codes.Unavailable, "engine unavailable")
	}
	affected, err := svc.Engine.SendSignal(ctx, req.SignalRef, protoToMap(req.Variables))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.SendSignalResponse{AffectedInstances: affected}, nil
}

// --- helpers ---

func protoToMap(m map[string]*structpb.Value) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v.AsInterface()
	}
	return out
}

func mapToProto(m map[string]any) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	if err != nil {
		return nil
	}
	return s
}

func loggingUnaryInterceptor(logger *observability.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil && logger != nil {
			logger.Error("grpc error", "method", info.FullMethod, "error", err)
		}
		return resp, err
	}
}
package handler

import "testing"

func TestRAGUploadJobCancelKeepsCompletedAndCancelsPending(t *testing.T) {
	store := newRAGUploadJobStore()
	jobID := store.create(1001, []*ragUploadResult{
		{FileName: "done.md", Status: ragUploadStatusCompleted, ChunkCount: 3},
		{FileName: "pending.md", Status: ragUploadStatusPending},
		{FileName: "running.md", Status: ragUploadStatusProcessing},
	}, nil, ragUploadOptions{})

	cancelled, ok := store.cancel(jobID, 1001, "用户取消上传任务")
	if !ok {
		t.Fatal("cancel returned false")
	}
	if cancelled.Status != ragUploadStatusCancelled {
		t.Fatalf("status=%q, want cancelled", cancelled.Status)
	}
	if cancelled.Completed != 1 || cancelled.Cancelled != 2 {
		t.Fatalf("completed=%d cancelled=%d, want 1/2", cancelled.Completed, cancelled.Cancelled)
	}
	if cancelled.Files[0].Status != ragUploadStatusCompleted {
		t.Fatalf("completed file status=%q, want completed", cancelled.Files[0].Status)
	}
	if cancelled.Files[1].Status != ragUploadStatusCancelled || cancelled.Files[2].Status != ragUploadStatusCancelled {
		t.Fatalf("pending/processing files not cancelled: %#v", cancelled.Files)
	}

	store.finish(jobID)
	after, _ := store.get(jobID, 1001)
	if after.Status != ragUploadStatusCancelled {
		t.Fatalf("finish overwrote cancelled status to %q", after.Status)
	}
}

func TestRAGUploadJobCancelAllSkipsTerminalJobs(t *testing.T) {
	store := newRAGUploadJobStore()
	active := store.create(1001, []*ragUploadResult{{FileName: "a.md", Status: ragUploadStatusPending}}, nil, ragUploadOptions{})
	done := store.create(1001, []*ragUploadResult{{FileName: "b.md", Status: ragUploadStatusCompleted}}, nil, ragUploadOptions{})
	store.finish(done)
	otherUser := store.create(2002, []*ragUploadResult{{FileName: "c.md", Status: ragUploadStatusPending}}, nil, ragUploadOptions{})

	count := store.cancelAll(1001, "用户取消全部上传任务")
	if count != 1 {
		t.Fatalf("cancelAll count=%d, want 1", count)
	}
	activeJob, _ := store.get(active, 1001)
	doneJob, _ := store.get(done, 1001)
	otherJob, _ := store.get(otherUser, 2002)
	if activeJob.Status != ragUploadStatusCancelled {
		t.Fatalf("active status=%q, want cancelled", activeJob.Status)
	}
	if doneJob.Status != ragUploadStatusCompleted {
		t.Fatalf("done status=%q, want completed", doneJob.Status)
	}
	if otherJob.Status != ragUploadStatusPending {
		t.Fatalf("other user status=%q, want pending", otherJob.Status)
	}
}

func TestRAGUploadJobRetryResetsFailedAndStalledFiles(t *testing.T) {
	store := newRAGUploadJobStore()
	jobID := store.create(1001, []*ragUploadResult{
		{FileName: "done.md", Status: ragUploadStatusCompleted, ChunkCount: 3},
		{FileName: "failed.md", Status: ragUploadStatusFailed, Msg: "rpc timeout"},
		{FileName: "stalled.md", Status: "stalled", Msg: "超过最大轮询次数"},
	}, []ragUploadWorkItem{
		{FileName: "done.md", Data: []byte("done")},
		{FileName: "failed.md", Data: []byte("retry me")},
		{FileName: "stalled.md", Data: []byte("retry stalled")},
	}, ragUploadOptions{Visibility: "private"})

	retry, ok := store.prepareRetry(jobID, 1001)
	if !ok {
		t.Fatal("prepareRetry returned false")
	}
	if len(retry.Items) != 3 || string(retry.Items[1].Data) != "retry me" || string(retry.Items[2].Data) != "retry stalled" {
		t.Fatalf("retry items not preserved for failed/stalled files: %#v", retry.Items)
	}
	if retry.Job.Status != ragUploadStatusPending || retry.Job.Failed != 0 {
		t.Fatalf("job after retry = status %q failed %d, want pending/0", retry.Job.Status, retry.Job.Failed)
	}
	if retry.Job.Files[0].Status != ragUploadStatusCompleted {
		t.Fatalf("completed file was reset: %#v", retry.Job.Files[0])
	}
	if retry.Job.Files[1].Status != ragUploadStatusPending || retry.Job.Files[2].Status != ragUploadStatusPending {
		t.Fatalf("failed/stalled files were not reset: %#v", retry.Job.Files)
	}
}

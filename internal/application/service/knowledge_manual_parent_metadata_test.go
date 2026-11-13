package service

import (
	"context"
	"encoding/json"
	"os"
	"sync/atomic"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestNativeKnowledgeManualParentWithCustomMetadata covers the deploy-only
// combination: a single manual POST provides both the native parent and
// custom metadata, which must land in the same Knowledge INSERT.
func TestNativeKnowledgeManualParentWithCustomMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	require.NoError(t, db.AutoMigrate(&types.Knowledge{}, &types.KnowledgeTag{}, &types.KnowledgeTagRelation{}))
	require.NoError(t, db.Migrator().DropColumn(&types.Knowledge{}, "ParentKnowledgeID"))
	ddl, err := os.ReadFile("../../../migrations/sqlite/000054_native_knowledge_parent.up.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(ddl)).Error)
	require.NoError(t, db.Exec(
		`CREATE TABLE knowledge_bases(id TEXT PRIMARY KEY,tenant_id BIGINT,deleted_at DATETIME);
		 INSERT INTO knowledge_bases VALUES ('kb',7,NULL)`,
	).Error)

	repo := repository.NewKnowledgeRepository(db)
	parent := &types.Knowledge{
		TenantID:        7,
		KnowledgeBaseID: "kb",
		Title:           "root",
		Type:            types.KnowledgeTypeManual,
		ParseStatus:     types.ParseStatusCompleted,
	}
	require.NoError(t, repo.CreateKnowledge(context.Background(), parent))

	var inserts int32
	countInsert := func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledges" {
			atomic.AddInt32(&inserts, 1)
		}
	}
	require.NoError(t, db.Callback().Create().Before("gorm:create").
		Register("test:count_knowledge_inserts", countInsert))

	kb := &types.KnowledgeBase{ID: "kb", TenantID: 7}
	kb.StorageProviderConfig = &types.StorageProviderConfig{Provider: "local"}
	queue := &createKnowledgeTaskEnqueuerStub{}
	svc := &knowledgeService{repo: repo, kbService: &writeKBLookup{kb: kb}, task: queue}
	ctx := types.WithExecutionTenant(context.Background(), 7)

	draftMeta := json.RawMessage(`{"source_id":"9007199254740993","version":"1"}`)
	draft, err := svc.CreateKnowledgeFromManual(ctx, "kb", &types.ManualKnowledgePayload{
		Title:             "child draft",
		Content:           "child draft body",
		Status:            "draft",
		ParentKnowledgeID: parent.ID,
		CustomMetadata:    draftMeta,
	}, "")
	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(&inserts), "manual create must use a single knowledge INSERT")
	require.Equal(t, parent.ID, draft.ParentKnowledgeID)
	require.JSONEq(t, string(draftMeta), string(draft.CustomMetadata))
	saved, err := repo.GetKnowledgeByID(ctx, 7, draft.ID)
	require.NoError(t, err)
	require.Equal(t, parent.ID, saved.ParentKnowledgeID)
	require.JSONEq(t, string(draftMeta), string(saved.CustomMetadata))

	atomic.StoreInt32(&inserts, 0)
	publishMeta := json.RawMessage(`{"source_id":"9007199254740994"}`)
	published, err := svc.CreateKnowledgeFromManual(ctx, "kb", &types.ManualKnowledgePayload{
		Title:             "child published",
		Content:           "child published body",
		Status:            "publish",
		ParentKnowledgeID: parent.ID,
		CustomMetadata:    publishMeta,
	}, "")
	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(&inserts))
	require.Equal(t, 1, queue.calls)
	require.Equal(t, "pending", published.ParseStatus)
	require.Equal(t, parent.ID, published.ParentKnowledgeID)
	saved, err = repo.GetKnowledgeByID(ctx, 7, published.ID)
	require.NoError(t, err)
	require.Equal(t, parent.ID, saved.ParentKnowledgeID)
	require.JSONEq(t, string(publishMeta), string(saved.CustomMetadata))

	var count int64
	require.NoError(t, db.Model(&types.Knowledge{}).Count(&count).Error)
	require.EqualValues(t, 3, count)
}

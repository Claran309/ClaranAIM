namespace go rag

struct RAGDocument {
    1: i64 id
    2: i64 owner_id
    3: string title
    4: string source
    5: string source_type
    6: string visibility
    7: i64 group_id
    8: i64 conversation_id
    9: string status
    10: string created_at
    11: string updated_at
    12: i64 chunk_count
}

struct RAGSource {
    1: i64 document_id
    2: i64 chunk_id
    3: string title
    4: string content
    5: string source
    6: double score
    7: string reason
}

struct RAGGraphNode {
    1: i64 id
    2: string name
    3: string type
    4: string summary
    5: i64 community_id
    6: double score
}

struct RAGGraphEdge {
    1: i64 id
    2: i64 source_id
    3: i64 target_id
    4: string relation
    5: double weight
    6: string evidence
    7: i64 document_id
}

struct RAGGraphCommunity {
    1: i64 id
    2: string name
    3: string summary
    4: i64 level
}

struct IngestDocumentReq {
    1: i64 owner_id
    2: string title
    3: string content
    4: string source
    5: string source_type
    6: string visibility
    7: i64 group_id
    8: i64 conversation_id
}

struct IngestDocumentResp {
    1: bool success
    2: RAGDocument document
    3: i64 chunk_count
    4: i64 entity_count
    5: i64 relation_count
    6: string msg
}

struct SearchReq {
    1: i64 viewer_id
    2: string query
    3: string mode
    4: i64 limit
    5: i64 group_id
    6: i64 conversation_id
    7: i64 document_id
}

struct SelfRAGCheckpoints {
    1: bool retrieve
    2: bool is_rel
    3: bool is_sup
    4: bool is_use
    5: string note
}

struct SearchResp {
    1: bool success
    2: string answer
    3: list<RAGSource> sources
    4: list<RAGGraphNode> graph_nodes
    5: list<RAGGraphEdge> graph_edges
    6: string route
    7: string crag_action
    8: SelfRAGCheckpoints self_check
    9: string msg
}

struct GraphReq {
    1: i64 viewer_id
    2: string query
    3: i64 limit
    4: i64 document_id
    5: i64 hops
}

struct GraphResp {
    1: bool success
    2: list<RAGGraphNode> nodes
    3: list<RAGGraphEdge> edges
    4: list<RAGGraphCommunity> communities
    5: string msg
}

struct ListDocumentsReq {
    1: i64 viewer_id
    2: i64 limit
    3: i64 offset
}

struct ListDocumentsResp {
    1: bool success
    2: list<RAGDocument> documents
    3: i64 total
    4: string msg
}

struct DeleteDocumentReq {
    1: i64 viewer_id
    2: i64 document_id
}

struct DeleteDocumentResp {
    1: bool success
    2: string msg
}

struct DeleteGraphReq {
    1: i64 viewer_id
    2: i64 document_id
}

struct DeleteGraphResp {
    1: bool success
    2: string msg
}

service RAGService {
    IngestDocumentResp IngestDocument(1: IngestDocumentReq req)
    SearchResp Search(1: SearchReq req)
    GraphResp GetGraph(1: GraphReq req)
    ListDocumentsResp ListDocuments(1: ListDocumentsReq req)
    DeleteDocumentResp DeleteDocument(1: DeleteDocumentReq req)
    DeleteGraphResp DeleteDocumentGraph(1: DeleteGraphReq req)
}

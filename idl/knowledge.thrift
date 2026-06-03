namespace go knowledge

struct KnowledgeGraphNode {
    1: i64 id
    2: string name
    3: string type
    4: string summary
    5: i64 community_id
    6: double score
    7: i64 degree
    8: double size
    9: string color
}

struct KnowledgeGraphEdge {
    1: i64 id
    2: i64 source_id
    3: i64 target_id
    4: string relation
    5: string description
    6: double weight
    7: string evidence
    8: string color
    9: i64 document_id
}

struct KnowledgeGraphCommunity {
    1: i64 id
    2: string name
    3: string summary
    4: i64 level
    5: string color
}

struct KnowledgeGraphStats {
    1: i64 node_count
    2: i64 edge_count
    3: i64 community_count
    4: list<string> types
    5: list<string> relations
}

struct KnowledgeGraphReq {
    1: i64 viewer_id
    2: string query
    3: list<string> type_filters
    4: list<string> relation_filters
    5: i64 community_id
    6: i64 hops
    7: i64 limit
    8: i64 document_id
}

struct KnowledgeGraphResp {
    1: bool success
    2: list<KnowledgeGraphNode> nodes
    3: list<KnowledgeGraphEdge> edges
    4: list<KnowledgeGraphCommunity> communities
    5: KnowledgeGraphStats stats
    6: string msg
}

struct KnowledgeNodeDetailReq {
    1: i64 viewer_id
    2: i64 node_id
    3: string query
    4: i64 limit
    5: i64 document_id
    6: i64 hops
}

struct KnowledgeNodeDetailResp {
    1: bool success
    2: KnowledgeGraphNode node
    3: list<KnowledgeGraphEdge> relations
    4: list<KnowledgeGraphNode> neighbors
    5: string msg
}

struct KnowledgeEdgeDetailReq {
    1: i64 viewer_id
    2: i64 edge_id
    3: string query
    4: i64 limit
    5: i64 document_id
    6: i64 hops
}

struct KnowledgeEdgeDetailResp {
    1: bool success
    2: KnowledgeGraphEdge edge
    3: KnowledgeGraphNode source
    4: KnowledgeGraphNode target
    5: string msg
}

struct KnowledgeNeighborhoodReq {
    1: i64 viewer_id
    2: i64 node_id
    3: string query
    4: list<string> type_filters
    5: list<string> relation_filters
    6: i64 community_id
    7: i64 hops
    8: i64 limit
    9: i64 document_id
}

struct KnowledgePathReq {
    1: i64 viewer_id
    2: i64 source_id
    3: i64 target_id
    4: string query
    5: i64 limit
    6: i64 document_id
    7: i64 hops
}

struct KnowledgePathResp {
    1: bool success
    2: list<KnowledgeGraphNode> nodes
    3: list<KnowledgeGraphEdge> edges
    4: list<i64> node_ids
    5: list<i64> edge_ids
    6: string msg
}

struct GraphReviewCandidate {
    1: i64 id
    2: string item_type
    3: i64 item_id
    4: string name
    5: string type
    6: string summary
    7: string evidence
    8: string reason
    9: string status
    10: string review_note
    11: string created_at
    12: string updated_at
    13: string reviewed_at
}

struct CreateGraphReviewCandidateReq {
    1: i64 viewer_id
    2: string item_type
    3: i64 item_id
    4: string reason
    5: string query
}

struct GraphReviewCandidateResp {
    1: bool success
    2: GraphReviewCandidate candidate
    3: string msg
}

struct ListGraphReviewCandidatesReq {
    1: i64 viewer_id
    2: string status
    3: string item_type
    4: i64 limit
    5: i64 offset
}

struct ListGraphReviewCandidatesResp {
    1: bool success
    2: list<GraphReviewCandidate> candidates
    3: i64 total
    4: string msg
}

struct ReviewGraphCandidateReq {
    1: i64 viewer_id
    2: i64 candidate_id
    3: string action
    4: string note
}

service KnowledgeService {
    KnowledgeGraphResp GetGraphView(1: KnowledgeGraphReq req)
    KnowledgeNodeDetailResp GetNodeDetail(1: KnowledgeNodeDetailReq req)
    KnowledgeEdgeDetailResp GetEdgeDetail(1: KnowledgeEdgeDetailReq req)
    KnowledgeGraphResp GetNeighborhood(1: KnowledgeNeighborhoodReq req)
    KnowledgePathResp GetPath(1: KnowledgePathReq req)
    GraphReviewCandidateResp CreateGraphReviewCandidate(1: CreateGraphReviewCandidateReq req)
    ListGraphReviewCandidatesResp ListGraphReviewCandidates(1: ListGraphReviewCandidatesReq req)
    GraphReviewCandidateResp ReviewGraphCandidate(1: ReviewGraphCandidateReq req)
}

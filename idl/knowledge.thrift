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
}

struct KnowledgePathReq {
    1: i64 viewer_id
    2: i64 source_id
    3: i64 target_id
    4: string query
    5: i64 limit
}

struct KnowledgePathResp {
    1: bool success
    2: list<KnowledgeGraphNode> nodes
    3: list<KnowledgeGraphEdge> edges
    4: list<i64> node_ids
    5: list<i64> edge_ids
    6: string msg
}

service KnowledgeService {
    KnowledgeGraphResp GetGraphView(1: KnowledgeGraphReq req)
    KnowledgeNodeDetailResp GetNodeDetail(1: KnowledgeNodeDetailReq req)
    KnowledgeEdgeDetailResp GetEdgeDetail(1: KnowledgeEdgeDetailReq req)
    KnowledgeGraphResp GetNeighborhood(1: KnowledgeNeighborhoodReq req)
    KnowledgePathResp GetPath(1: KnowledgePathReq req)
}

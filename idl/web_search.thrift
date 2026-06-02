namespace go web_search

struct WebSearchSource {
    1: string title
    2: string url
    3: string snippet
    4: string source
    5: bool trusted
    6: double score
    7: string fetch_status
    8: list<string> passages
}

struct SearchReq {
    1: string query
    2: i64 limit
}

struct SearchResp {
    1: bool success
    2: string query
    3: list<WebSearchSource> results
    4: string msg
}

struct AugmentReq {
    1: string query
    2: i64 limit
    3: i64 max_fetch
    4: i64 max_passages
}

struct AugmentResp {
    1: bool success
    2: string query
    3: string answer_context
    4: list<WebSearchSource> sources
    5: string search_time
    6: string msg
}

service WebSearchService {
    SearchResp Search(1: SearchReq req)
    AugmentResp Augment(1: AugmentReq req)
}

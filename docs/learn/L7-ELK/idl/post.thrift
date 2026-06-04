namespace go post

struct CreatePostReq{
    1: i64 userID
    2: string postName
    3: string content
}

struct CreatePostResp{
    1: bool success
    2: i64 postID
    3: string msg
}

struct DeletePostReq{
    1: i64 userID
    2: i64 postID
}

struct DeletePostResp{
    1: bool success
    2: string msg
}

service PostService{
    CreatePostResp CreatePost(1: CreatePostReq req)
    DeletePostResp DeletePost(1: DeletePostReq req)
}
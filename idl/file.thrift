namespace go file

struct UploadFileReq {
    1: string file_name
    2: string file_type
    3: i64 file_size
    4: string content_type
    5: i64 uploader_id
    6: string file_url
}

struct UploadFileResp {
    1: bool success
    2: string file_url
    3: string file_id
    4: string msg
}

struct GetFileReq {
    1: string file_id
}

struct GetFileResp {
    1: bool success
    2: string file_url
    3: string file_name
    4: string file_type
    5: i64 file_size
    6: string content_type
    7: string msg
}

struct DeleteFileReq {
    1: string file_id
    2: i64 operator_id
}

struct DeleteFileResp {
    1: bool success
    2: string msg
}

struct FileInfo {
    1: string file_id
    2: string file_name
    3: string file_type
    4: i64 file_size
    5: string content_type
    6: string file_url
    7: i64 uploader_id
    8: string created_at
}

struct ListFilesReq {
    1: i64 uploader_id
    2: string file_type
    3: i64 limit
    4: i64 offset
}

struct ListFilesResp {
    1: bool success
    2: list<FileInfo> files
    3: i64 total
    4: string msg
}

service FileService {
    UploadFileResp UploadFile(1: UploadFileReq req)
    GetFileResp GetFile(1: GetFileReq req)
    DeleteFileResp DeleteFile(1: DeleteFileReq req)
    ListFilesResp ListFiles(1: ListFilesReq req)
}

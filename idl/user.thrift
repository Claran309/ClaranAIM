namespace go user

struct User {
    1: i64 id
    2: string username
    3: string nickname
    4: string avatar
    5: string email
    6: string phone
    7: string status
    8: string created_at
    9: string updated_at
    10: string cover
    11: string signature
    12: string bio
    13: string location
    14: string website
    15: string gender
    16: string birthday
    17: string role
    18: bool is_system
}

struct RegisterReq {
    1: string username
    2: string password
    3: string nickname
    4: bool is_system
}

struct RegisterResp {
    1: bool success
    2: i64 user_id
    3: string msg
}

struct LoginReq {
    1: string username
    2: string password
}

struct LoginResp {
    1: bool success
    2: string token
    3: i64 user_id
    4: string msg
    5: string access_token
    6: string refresh_token
    7: string role
}

struct GetUserInfoReq {
    1: i64 user_id
}

struct GetUserInfoResp {
    1: bool success
    2: User user
    3: string msg
}

struct UpdateUserInfoReq {
    1: i64 user_id
    2: string nickname
    3: string email
    4: string phone
    5: string avatar
    6: string cover
    7: string signature
    8: string bio
    9: string location
    10: string website
    11: string gender
    12: string birthday
    13: bool full_update
}

struct UpdateUserInfoResp {
    1: bool success
    2: string msg
}

struct UpdateAvatarReq {
    1: i64 user_id
    2: string avatar
}

struct UpdateAvatarResp {
    1: bool success
    2: string msg
}

struct UpdateStatusReq {
    1: i64 user_id
    2: string status
}

struct UpdateStatusResp {
    1: bool success
    2: string msg
}

struct AddFriendReq {
    1: i64 user_id
    2: i64 friend_id
    3: i64 group_id
    4: string remark
}

struct AddFriendResp {
    1: bool success
    2: string msg
}

struct DeleteFriendReq {
    1: i64 user_id
    2: i64 friend_id
}

struct DeleteFriendResp {
    1: bool success
    2: string msg
}

struct Friend {
    1: i64 id
    2: i64 user_id
    3: i64 friend_id
    4: i64 group_id
    5: string remark
    6: string friend_name
    7: string friend_avatar
    8: string friend_status
    9: string group_name
}

struct GetFriendListReq {
    1: i64 user_id
}

struct GetFriendListResp {
    1: bool success
    2: list<Friend> friends
    3: string msg
}

struct UpdateFriendRemarkReq {
    1: i64 user_id
    2: i64 friend_id
    3: string remark
    4: i64 group_id
}

struct UpdateFriendRemarkResp {
    1: bool success
    2: string msg
}

struct FriendGroup {
    1: i64 id
    2: i64 user_id
    3: string name
}

struct CreateFriendGroupReq {
    1: i64 user_id
    2: string name
}

struct CreateFriendGroupResp {
    1: bool success
    2: i64 group_id
    3: string msg
}

struct MoveFriendGroupReq {
    1: i64 user_id
    2: i64 friend_id
    3: i64 group_id
}

struct MoveFriendGroupResp {
    1: bool success
    2: string msg
}

struct GetFriendGroupsReq {
    1: i64 user_id
}

struct GetFriendGroupsResp {
    1: bool success
    2: list<FriendGroup> groups
    3: string msg
}

struct BatchGetUserInfoReq {
    1: list<i64> user_ids
}

struct BatchGetUserInfoResp {
    1: bool success
    2: list<User> users
    3: string msg
}

struct AdminListUsersReq {
    1: string keyword
    2: string role
    3: string status
    4: bool include_system
    5: i64 limit
    6: i64 offset
}

struct AdminListUsersResp {
    1: bool success
    2: list<User> users
    3: i64 total
    4: string msg
}

service UserService {
    RegisterResp Register(1: RegisterReq req)
    LoginResp Login(1: LoginReq req)
    GetUserInfoResp GetUserInfo(1: GetUserInfoReq req)
    UpdateUserInfoResp UpdateUserInfo(1: UpdateUserInfoReq req)
    UpdateAvatarResp UpdateAvatar(1: UpdateAvatarReq req)
    UpdateStatusResp UpdateStatus(1: UpdateStatusReq req)
    AddFriendResp AddFriend(1: AddFriendReq req)
    DeleteFriendResp DeleteFriend(1: DeleteFriendReq req)
    GetFriendListResp GetFriendList(1: GetFriendListReq req)
    UpdateFriendRemarkResp UpdateFriendRemark(1: UpdateFriendRemarkReq req)
    CreateFriendGroupResp CreateFriendGroup(1: CreateFriendGroupReq req)
    MoveFriendGroupResp MoveFriendGroup(1: MoveFriendGroupReq req)
    GetFriendGroupsResp GetFriendGroups(1: GetFriendGroupsReq req)
    BatchGetUserInfoResp BatchGetUserInfo(1: BatchGetUserInfoReq req)
    AdminListUsersResp AdminListUsers(1: AdminListUsersReq req)
}

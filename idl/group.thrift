namespace go group

struct Group {
    1: i64 id
    2: string name
    3: string avatar
    4: i64 owner_id
    5: string announcement
    6: string created_at
    7: string updated_at
    8: bool is_pinned
}

struct GroupMember {
    1: i64 id
    2: i64 group_id
    3: i64 user_id
    4: string role
    5: string muted_until
    6: string joined_at
    7: string username
    8: string avatar
}

struct CreateGroupReq {
    1: string name
    2: i64 owner_id
    3: list<i64> member_ids
}

struct CreateGroupResp {
    1: bool success
    2: i64 group_id
    3: string msg
}

struct DeleteGroupReq {
    1: i64 group_id
    2: i64 operator_id
}

struct DeleteGroupResp {
    1: bool success
    2: string msg
}

struct UpdateGroupReq {
    1: i64 group_id
    2: i64 operator_id
    3: string name
    4: string announcement
}

struct UpdateGroupResp {
    1: bool success
    2: string msg
}

struct GetGroupReq {
    1: i64 group_id
}

struct GetGroupResp {
    1: bool success
    2: Group group
    3: string msg
}

struct GetUserGroupsReq {
    1: i64 user_id
}

struct GetUserGroupsResp {
    1: bool success
    2: list<Group> groups
    3: string msg
}

struct InviteMemberReq {
    1: i64 group_id
    2: i64 operator_id
    3: list<i64> user_ids
}

struct InviteMemberResp {
    1: bool success
    2: string msg
}

struct KickMemberReq {
    1: i64 group_id
    2: i64 operator_id
    3: i64 user_id
}

struct KickMemberResp {
    1: bool success
    2: string msg
}

struct MuteMemberReq {
    1: i64 group_id
    2: i64 operator_id
    3: i64 user_id
    4: i64 duration_minutes
}

struct MuteMemberResp {
    1: bool success
    2: string msg
}

struct UnmuteMemberReq {
    1: i64 group_id
    2: i64 operator_id
    3: i64 user_id
}

struct UnmuteMemberResp {
    1: bool success
    2: string msg
}

struct SetRoleReq {
    1: i64 group_id
    2: i64 operator_id
    3: i64 user_id
    4: string role
}

struct SetRoleResp {
    1: bool success
    2: string msg
}

struct GetGroupMembersReq {
    1: i64 group_id
}

struct GetGroupMembersResp {
    1: bool success
    2: list<GroupMember> members
    3: string msg
}

struct CheckMemberReq {
    1: i64 group_id
    2: i64 user_id
}

struct CheckMemberResp {
    1: bool success
    2: bool is_member
    3: string role
    4: string msg
}

struct TransferOwnerReq {
    1: i64 group_id
    2: i64 operator_id
    3: i64 new_owner_id
}

struct TransferOwnerResp {
    1: bool success
    2: string msg
}

struct PinGroupReq {
    1: i64 group_id
    2: i64 operator_id
    3: bool is_pinned
}

struct PinGroupResp {
    1: bool success
    2: string msg
}

service GroupService {
    CreateGroupResp CreateGroup(1: CreateGroupReq req)
    DeleteGroupResp DeleteGroup(1: DeleteGroupReq req)
    UpdateGroupResp UpdateGroup(1: UpdateGroupReq req)
    GetGroupResp GetGroup(1: GetGroupReq req)
    GetUserGroupsResp GetUserGroups(1: GetUserGroupsReq req)
    InviteMemberResp InviteMember(1: InviteMemberReq req)
    KickMemberResp KickMember(1: KickMemberReq req)
    MuteMemberResp MuteMember(1: MuteMemberReq req)
    UnmuteMemberResp UnmuteMember(1: UnmuteMemberReq req)
    SetRoleResp SetRole(1: SetRoleReq req)
    GetGroupMembersResp GetGroupMembers(1: GetGroupMembersReq req)
    CheckMemberResp CheckMember(1: CheckMemberReq req)
    TransferOwnerResp TransferOwner(1: TransferOwnerReq req)
    PinGroupResp PinGroup(1: PinGroupReq req)
}

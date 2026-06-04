namespace go user

struct RegisterReq{
    1: string username
    2: string password
    3: string email
}

struct RegisterResp{
    1: bool success
    2: i64 userID
    3: string msg
}

struct LoginByUsernameReq{
    1: string username
    2: string password
}

struct LoginByUsernameResp{
    1: bool success
    2: string token
    3: string msg
}

service UserService{
    RegisterResp Register(1: RegisterReq req)
    LoginByUsernameResp Login(1: LoginByUsernameReq req)
}
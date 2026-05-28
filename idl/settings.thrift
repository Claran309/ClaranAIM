namespace go settings

struct LLMProfile {
    1: i64 id
    2: string name
    3: string provider_type
    4: string base_url
    5: string model_name
    6: string usage_type
    7: bool is_default
    8: bool enabled
    9: bool has_api_key
}

struct SaveLLMProfileReq {
    1: i64 user_id
    2: i64 id
    3: string name
    4: string provider_type
    5: string base_url
    6: string api_key
    7: string model_name
    8: string usage_type
    9: bool is_default
    10: bool enabled
    11: string api_key_action
}

struct SaveLLMProfileResp {
    1: bool success
    2: LLMProfile profile
    3: string msg
}

struct ListLLMProfilesReq {
    1: i64 user_id
    2: string usage_type
}

struct ListLLMProfilesResp {
    1: bool success
    2: list<LLMProfile> profiles
    3: string msg
}

struct PromptTemplate {
    1: i64 id
    2: string type
    3: string name
    4: string content
    5: bool is_default
    6: bool enabled
}

struct SavePromptReq {
    1: i64 user_id
    2: i64 id
    3: string type
    4: string name
    5: string content
    6: bool is_default
    7: bool enabled
}

struct SavePromptResp {
    1: bool success
    2: PromptTemplate prompt
    3: string msg
}

struct ListPromptsReq {
    1: i64 user_id
}

struct ListPromptsResp {
    1: bool success
    2: list<PromptTemplate> prompts
    3: string msg
}

struct ResolveLLMProfileReq {
    1: i64 user_id
    2: i64 profile_id
}

struct ResolveLLMProfileResp {
    1: bool success
    2: string api_key
    3: string base_url
    4: string model_name
    5: string provider_type
    6: string msg
}

service SettingsService {
    SaveLLMProfileResp SaveLLMProfile(1: SaveLLMProfileReq req)
    ListLLMProfilesResp ListLLMProfiles(1: ListLLMProfilesReq req)
    SavePromptResp SavePrompt(1: SavePromptReq req)
    ListPromptsResp ListPrompts(1: ListPromptsReq req)
    ResolveLLMProfileResp ResolveLLMProfile(1: ResolveLLMProfileReq req)
}

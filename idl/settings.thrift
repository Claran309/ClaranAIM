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
    12: bool enabled_set
}

struct DeleteLLMProfileReq {
    1: i64 user_id
    2: i64 profile_id
}

struct DeleteLLMProfileResp {
    1: bool success
    2: string msg
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
    8: bool enabled_set
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
    2: i64 profile_id
    3: string api_key
    4: string base_url
    5: string model_name
    6: string provider_type
    7: string prompt_template
    8: string msg
}

struct ResolveTranslationConfigReq {
    1: i64 user_id
}

struct ResolveTranslationConfigResp {
    1: bool success
    2: i64 profile_id
    3: string api_key
    4: string base_url
    5: string model_name
    6: string provider_type
    7: string prompt_template
    8: string msg
}

struct SkillFile {
    1: string path
    2: binary content
}

struct AgentSkill {
    1: i64 id
    2: i64 owner_id
    3: i64 agent_id
    4: string scope
    5: string name
    6: string description
    7: string skills_dir
    8: string entry_file
    9: string source_type
    10: bool is_default
    11: bool enabled
    12: string summary
    13: string content
}

struct SaveSkillReq {
    1: i64 user_id
    2: i64 id
    3: string name
    4: string description
    5: string scope
    6: i64 agent_id
    7: string file_name
    8: binary content
    9: list<SkillFile> files
    10: bool is_default
    11: bool enabled
    12: bool enabled_set
}

struct SaveSkillResp {
    1: bool success
    2: AgentSkill skill
    3: string msg
}

struct ListSkillsReq {
    1: i64 user_id
    2: string scope
    3: i64 agent_id
}

struct ListSkillsResp {
    1: bool success
    2: list<AgentSkill> skills
    3: string msg
}

struct GetSkillReq {
    1: i64 user_id
    2: i64 skill_id
}

struct GetSkillResp {
    1: bool success
    2: AgentSkill skill
    3: string msg
}

struct UpdateSkillContentReq {
    1: i64 user_id
    2: i64 skill_id
    3: string name
    4: string description
    5: binary content
}

struct UpdateSkillContentResp {
    1: bool success
    2: AgentSkill skill
    3: string msg
}

struct DeleteSkillReq {
    1: i64 user_id
    2: i64 skill_id
}

struct DeleteSkillResp {
    1: bool success
    2: string msg
}

service SettingsService {
    SaveLLMProfileResp SaveLLMProfile(1: SaveLLMProfileReq req)
    ListLLMProfilesResp ListLLMProfiles(1: ListLLMProfilesReq req)
    DeleteLLMProfileResp DeleteLLMProfile(1: DeleteLLMProfileReq req)
    SavePromptResp SavePrompt(1: SavePromptReq req)
    ListPromptsResp ListPrompts(1: ListPromptsReq req)
    ResolveTranslationConfigResp ResolveTranslationConfig(1: ResolveTranslationConfigReq req)
    ResolveLLMProfileResp ResolveLLMProfile(1: ResolveLLMProfileReq req)
    SaveSkillResp SaveSkill(1: SaveSkillReq req)
    ListSkillsResp ListSkills(1: ListSkillsReq req)
    GetSkillResp GetSkill(1: GetSkillReq req)
    UpdateSkillContentResp UpdateSkillContent(1: UpdateSkillContentReq req)
    DeleteSkillResp DeleteSkill(1: DeleteSkillReq req)
}

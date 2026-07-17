package marketplace

// ChannelName identifies the in-process marketplace bridge in the model
// catalog. The routable model list is dynamic (one entry per enabled script →
// model binding), so GetModelList reads it from the binding table at runtime.
const ChannelName = "AiToken Marketplace"

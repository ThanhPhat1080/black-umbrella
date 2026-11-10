import { HubDO, type Env } from "./hub";

export { HubDO };

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    return env.HUB.get(env.HUB.idFromName("hub")).fetch(req);
  },
};

# slack-multi-channel-invite
Have you ever googled `"slack invite user to multiple channels"`?  Yeah, me too.  I do this every time a new engineer joins my team, and I inevitably end up inviting said engineer to each Slack channel manually.  I got tired of this, so I rolled up my sleeves and whipped up this script.

I assume Slack will eventually add this ability.  Until then, hopefully you can save some time by using this.

Enjoy!

## Instructions
1. [Create](https://api.slack.com/apps) a Slack App for your workspace.
2. Add the following permission scopes to a user token (bot tokens aren't allowed `channels:write`):
    - `users:read`
    - `users:read.email`
    - `channels:read`
    - `channels:write`
    - `groups:read` (only if inviting to private channels)
    - `groups:write` (only if inviting to private channels)
3. Install app to your workspace which will generate a new User OAuth token
4. Download script:
    - If you have Go installed: `go install github.com/jamietsao/slack-multi-channel-invite@latest`
    - Else download the binary directly: https://github.com/jamietsao/slack-multi-channel-invite/releases
5. Run script:

`slack-multi-channel-invite -api_token=<user-oauth-token> -emails=steph@warriors.com,klay@warriors.com -channels=dubnation,splashbrothers,thetown -private=<true|false>`

The users with emails `steph@warriors.com` and `klay@warriors.com` should be invited to channels `dubnation`, `splashbrothers`, and `thetown`!

_* Set `private` flag to `true` if you want to invite users to private channels.  As noted above, this will require the additional permission scopes of `groups:read` and `groups:write`_

#### Want to remove users from channels?
Simply set the optional `action` flag to `remove` (`add` is the default):

`slack-multi-channel-invite -api_token=<user-oauth-token> -action=remove -emails=kd@warriors.com -channels=dubnation,warriors -private=<true|false>`


## Implementation
Initially, I figured this script would be a simple loop that invoked some API to invite users to a channel.  It turns out this API endpoint ([`conversations.invite`](https://api.slack.com/methods/conversations.invite)) expects the user ID (instead of username) and channel ID (instead of channel name).  Problem is, it's not very straightforward to get user and channel IDs. There isn't a way to lookup a user by username (only by email).  And there's no way to look up a single channel, unless you have the channel ID already (chicken and egg).

For these reasons, I wrote the script like so:
1. [Look up](https://api.slack.com/methods/users.lookupByEmail) Slack user IDs for all given emails.
2. [Query](https://api.slack.com/methods/conversations.list) all public (or private) channels in the workspace and create a name -> ID mapping.
3. For each of the given channels, [invite](https://api.slack.com/methods/conversations.invite) the users to the channel using the user IDs and channel ID from steps 1 & 2.

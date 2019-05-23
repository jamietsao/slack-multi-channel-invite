# slack-multi-channel-invite
Have you ever googled `"slack invite user to multiple channels"`?  Yeah, me too.  I do this every time a new engineer joins my team, and inevitably end up inviting said engineer to each Slack channel manually.  I got tired of this, so I rolled up my sleeves and whipped up this script in a couple of hours.

I assume Slack will eventually add this ability.  Until then, hopefully you can save some time by using this script.

Enjoy!

## Instructions
1. [Create](https://api.slack.com/apps) a Slack App for your workspace.
2. Add the following permission scopes to your app:
    - `users.read`
    - `users.read.email`
    - `channels.read`
    - `channels.write`
3. Install app to your workspace which will generate a new OAuth Access Token
4. Download script:
    - If you have Go installed: `go get github.com/jamietsao/slack-multi-channel-invite`
    - Else download the binary directly: https://github.com/jamietsao/slack-multi-channel-invite/releases
5. Run script:

`slack-multi-channel-invite -api_token=<oauth-access-token> -channels=foo,bar,baz -user_email=steph@curry.com`

The user with email `steph@curry.com` should be invited to channels `foo`, `bar`, and `baz`!

## Implementation
Initially, I figured this script would be a simple loop that invoked some API to invite a user to a channel.  It turns out this API endpoint ([`conversations.invite`](https://api.slack.com/methods/conversations.invite)) expects the user ID (instead of username) and channel ID (instead of channel name). Furthermore, there isn't a way to lookup a user by username (only by email).  And there's no way to look up a single channel, except by channel ID (chicken and egg).

For these reasons, I wrote the script like so:
1. [Look up](https://api.slack.com/methods/users.lookupByEmail) the Slack user ID by email.
2. [Query](https://api.slack.com/methods/conversations.list) all public channels in the workspace and create a name -> ID mapping.
3. For each of the given channels, [invite](https://api.slack.com/methods/conversations.invite) the user to the channel using the user ID and channel ID from steps 1 & 2.

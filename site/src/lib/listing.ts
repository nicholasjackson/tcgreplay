// Whether TCG Replay is listed in ChatGPT's Plugin Directory, and the name
// to search for there. While `listed` is false, the connecting page
// (pages/docs/connect.astro) gives only the custom connection steps and no
// page may mention the Plugin Directory; the browser suite checks both.
//
// Set `listed` to true only after OpenAI has approved the listing and the
// maintainer chooses to switch: the connecting page then starts its ChatGPT
// section with the directory steps, and the browser suite follows.
export const chatgptDirectory = { listed: false, name: 'TCG Replay' } as const;

// The ChatGPT plans that allow adding a custom MCP server, as the
// connecting page names them. Checked against OpenAI's help and a real
// account at launch.
export const chatgptPlans = 'Business, Enterprise or Edu';

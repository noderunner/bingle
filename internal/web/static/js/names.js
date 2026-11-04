// The name a guest plays under until they choose one: two words, to be easy to say
// and to remember ("Mossy Otter"). The space means it can never be an account's
// handle. 96 words of each kind make over 9,000 names, which is plenty for telling the
// players of a table apart, and a table that does collide just refuses the second.

const FIRST = [
	"Amber", "Brave", "Breezy", "Bright", "Bubbly", "Calm", "Cheeky", "Cheery", "Clever", "Cloudy", "Cobalt", "Copper",
	"Cosmic", "Cozy", "Crimson", "Curious", "Daring", "Dapper", "Dizzy", "Dreamy", "Dusty", "Eager", "Electric", "Fancy",
	"Feisty", "Fizzy", "Fluffy", "Frosty", "Fuzzy", "Gentle", "Giddy", "Golden", "Grumpy", "Happy", "Hasty", "Hazy",
	"Humble", "Icy", "Jazzy", "Jolly", "Jumpy", "Kindly", "Lively", "Lucky", "Lunar", "Marble", "Mellow", "Merry",
	"Mighty", "Misty", "Modest", "Mossy", "Nifty", "Nimble", "Noble", "Peppy", "Perky", "Plucky", "Polite", "Proud",
	"Quick", "Quiet", "Quirky", "Rapid", "Rosy", "Rusty", "Salty", "Sandy", "Scarlet", "Shiny", "Silent", "Silly",
	"Silver", "Sleepy", "Smoky", "Snappy", "Sneaky", "Snowy", "Solar", "Speedy", "Spicy", "Spry", "Starry", "Stormy",
	"Sunny", "Swift", "Tidy", "Tiny", "Toasty", "Velvet", "Wandering", "Whimsical", "Windy", "Witty", "Wobbly", "Zesty",
];

const SECOND = [
	"Acorn", "Alpaca", "Anchor", "Badger", "Banjo", "Beacon", "Beetle", "Biscuit", "Bison", "Bobcat", "Bumblebee", "Button",
	"Cactus", "Capybara", "Cedar", "Comet", "Cricket", "Crumpet", "Daisy", "Dingo", "Dolphin", "Dumpling", "Falcon", "Ferret",
	"Fern", "Finch", "Firefly", "Fox", "Gecko", "Gnome", "Goose", "Hedgehog", "Heron", "Hippo", "Hopper", "Ibex",
	"Iguana", "Jackal", "Jellyfish", "Kettle", "Kiwi", "Koala", "Ladybug", "Lantern", "Lemur", "Llama", "Lobster", "Lynx",
	"Magpie", "Mango", "Maple", "Marmot", "Meadow", "Meerkat", "Mitten", "Mongoose", "Moose", "Muffin", "Narwhal", "Newt",
	"Noodle", "Nutmeg", "Ocelot", "Octopus", "Otter", "Owl", "Panda", "Pebble", "Pelican", "Penguin", "Pickle", "Pigeon",
	"Pine", "Platypus", "Puffin", "Quokka", "Raccoon", "Radish", "Raven", "Robin", "Rocket", "Salamander", "Sparrow", "Sprout",
	"Squid", "Stoat", "Teapot", "Thistle", "Toad", "Toucan", "Truffle", "Turnip", "Walrus", "Waffle", "Weasel", "Wombat",
];

const pick = (list) => list[Math.floor(Math.random() * list.length)];

/** A new name for a guest. */
export function funName() {
	return `${pick(FIRST)} ${pick(SECOND)}`;
}

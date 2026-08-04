const { sharedHelper } = require('./util');

function process() {
    return sharedHelper(2);
}

module.exports = { process };

function collide() {
    return 'b';
}

function useB() {
    return collide();
}

module.exports = { collide, useB };

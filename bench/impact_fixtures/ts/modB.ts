export function collide(): string {
    return 'b';
}

export function useB(): string {
    return collide();
}

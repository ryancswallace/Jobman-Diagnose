fn select_partition(partitions: &[u8], selected: usize) -> u8 {
    partitions[selected]
}

fn main() {
    let partitions = [4_u8, 7_u8];
    eprintln!("selecting partition index 3 from {} entries", partitions.len());
    println!("{}", select_partition(&partitions, 3));
}

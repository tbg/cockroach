extern crate bio;
extern crate hlc;

use std::ops::Range;

use self::bio::data_structures::interval_tree::IntervalTree;
use self::bio::utils::Interval;

use self::hlc::{HLTimespec, State};


type Key = Vec<u8>;

enum TxnID {
    None,
    UUID([u8; 16]),
}

struct Data {
    ts: HLTimespec,
    txn: TxnID,
}

struct RangeData<T>(Range<T>, Data);

impl<'a> RangeData<Key> {
    pub fn from_args(start: Vec<u8>,
                     end: Vec<u8>,
                     ts: HLTimespec,
                     txnID: TxnID)
                     -> RangeData<Key> {
        RangeData(Range {start, end},
                  Data {
                      ts: HLTimespec::new(1, 0, 0),
                      txn: TxnID::None,
                  })
    }
}

struct TimestampCache {
    // NB: This does not yet allow deletion, though that should be
    // straightforward.
    //
    // See https://github.com/rust-bio/rust-bio/issues/141.
    tree: IntervalTree<Key, Data>,
}

impl TimestampCache {
    pub fn new() -> TimestampCache {
        TimestampCache { tree: IntervalTree::new() }
    }

    pub fn insert(&mut self, k: Range<Key>, d: Data) {
        self.tree.insert(k, d)
    }

    pub fn max(&self, interval: Range<Key>) -> HLTimespec {
        self.tree.find(interval).map(|entry| entry.data().ts)
                                .max()
                                .unwrap_or(HLTimespec::new(0,0,0))
    }
}

mod tests {
    use super::*;

    #[test]
    fn interval_basic() {
        let inserts = vec![
            RangeData::from_args(vec![0], vec![80], HLTimespec::new(10, 0, 0), TxnID::None),
            RangeData::from_args(vec![80], vec![99], HLTimespec::new(5, 0, 0), TxnID::None),
            RangeData::from_args(vec![75], vec![85], HLTimespec::new(11, 0, 0), TxnID::None),
        ];

        let mut tsc = TimestampCache::new();
        for rd in inserts {
            tsc.insert(rd.0, rd.1);
        }

        let tests = vec![
            (vec![0],  vec![75], HLTimespec::new(10,0,0), TxnID::None),
            (vec![1],  vec![76], HLTimespec::new(11,0,0), TxnID::None),
            (vec![1],  vec![99], HLTimespec::new(11,0,0), TxnID::None),
            (vec![85], vec![99], HLTimespec::new(11,0,0), TxnID::None),
            (vec![86], vec![99], HLTimespec::new(5,0,0), TxnID::None),
            (vec![90], vec![200], HLTimespec::new(5,0,0), TxnID::None),
        ];

        for test in tests {
            let rd = RangeData::from_args(test.0, test.1, test.2, test.3);
            let max_ts = tsc.max(rd.0);
            assert_eq!(rd.1.ts, max_ts);
        }
    }

    #[test]
    fn double_insertion() {
        let mut tree = IntervalTree::new();
        tree.insert(11..20, "Range_1");
        tree.insert(11..20, "Range_1 again");

        let mut count = 0;
        for r in tree.find(15..25) {
            assert_eq!(r.interval().start, 11);
            assert_eq!(r.interval().end, 20);
            assert_eq!(r.interval(), &(Interval::from(11..20)));
            count += 1;
        }
        assert_eq!(count, 2);
    }
}
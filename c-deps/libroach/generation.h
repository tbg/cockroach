#include "timestamp.h"

namespace cockroach {

enum generation { noGeneration, active, passive };
enum versionState { noVersion, meta, metaVersion, firstVersion, shadowed };

// genHelper is fed information from an (active-generation) iterator during an
// MVCC read operation which it uses to decide which versioned values are
// eligible for being moved to the passive generation.
class genHelper {
 public:
  genHelper(DBTimestamp cutoff)
      : cutoff_(cutoff),
        // TODO(tschottdorf): allocate on first use instead?
        ops_(new rocksdb::WriteBatch),
        max_move_ts_(cockroach::kZeroTimestamp),
        gen_(noGeneration),
        state_(noVersion){};

  // first is called when the top level record of a new of a new key is visited.
  // is_value is true if and only if there is no intent on that key.
  void first(bool is_value) {
    state_ = is_value ? firstVersion : meta;
    gen_ = active;
    fprintf(stderr, "first %d, state=%d\n", is_value, state_);
  };
  // move_to signals that the indicator has been moved to the next version (whose raw key bytes
  // and timestamp are supplied).
  void move_to(rocksdb::Slice raw_key, DBTimestamp ts) {
    fprintf(stderr, "move_to %s %lld\n", raw_key.ToString().c_str(), ts.wall_time);
    next();
    move(raw_key, ts);
  }

 private:
  // next is called to signal that the iterator has moved to (at least) the next versioned key,
  // which affects whether future versions are shadowed.
  void next() {
    switch (state_) {
    case meta:
      state_ = metaVersion;
    case metaVersion:
      state_ = firstVersion;
    case firstVersion:
      state_ = shadowed;
    case shadowed:
      break;
    default:
      abort();
    }
    fprintf(stderr, "state is now %d\n", state_);
  }

  // move offers a versioned value for moving to the passive keyspace (if it is permanently
  // shadowed and old enough).
  void move(rocksdb::Slice raw_key, DBTimestamp ts) {
    if (gen_ != active) {
      // Don't move a key that is already in the passive generation.
      fprintf(stderr, "not active");
      return;
    }
    if (state_ != shadowed) {
      // Don't move a key that is not shadowed. Anything that is live or could be
      // live again in the future must be in the active generation.
      fprintf(stderr, "not shadowed");
      return;
    }

    if (ts == cockroach::kZeroTimestamp || ts > cutoff_) {
      // Don't move keys that are inline or recent.
      fprintf(stderr, "ts=%lld cutoff=%lld\n", ts.wall_time, cutoff_.wall_time);
      return;
    }

    if (ts > max_move_ts_) {
      max_move_ts_ = ts;
    }

    // TODO(tschottdorf): actually move to a passive generation.
    fprintf(stderr, "deleting");
    ops_->Delete(raw_key);
  };

  // cutoff_ is the timestamp at and below which keys will be moved into the passive
  // generation.
  DBTimestamp cutoff_;

 public:
  // ops_ is a batch that, when applied, moves all collected versioned keys into the
  // passive keyspace.
  // TODO(tschottdorf): think through the contract here. For now, the WriteBatch simply
  // contains deletions (i.e. simulates case in which passive keyspace is never read).
  std::unique_ptr<rocksdb::WriteBatch> ops_;
  // max_move_ts_ is the maximum timestamp over all keys affected in ops_. Once
  // ops_ is applied, all reads at timestamps <= max_move_ts must consult the
  // passive generation.
  DBTimestamp max_move_ts_;
  // gen_ holds the generation the current key came from. This is relevant a)
  // when emitting the key (there may be a prefix we must strip before returning
  // the key to the caller) and b) when deciding whether to move the key into
  // the passive generation, which we only want to do if it isn't already there.
  generation gen_;
  // state_ tracks whether keys we're observing now are permanently shadowed
  // (i.e. covered by a newer, fully committed version).
  versionState state_;
};

}  // namespace cockroach

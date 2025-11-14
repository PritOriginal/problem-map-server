[out:xml];
area["name"="Тамбовская область"]["boundary"="administrative"]->.a;
(
  nwr["place"~"city|town|municipality|village"](area.a);
);
(._;>;);
out;

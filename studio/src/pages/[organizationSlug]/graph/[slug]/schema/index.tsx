import { EmptyState } from "@/components/empty-state";
import { getGraphLayout } from "@/components/layout/graph-layout";
import { PageHeader } from "@/components/layout/head";
import { TitleLayout } from "@/components/layout/title-layout";
import { SchemaToolbar } from "@/components/schema/toolbar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Loader } from "@/components/ui/loader";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { NextPageWithLayout } from "@/lib/page";
import {
  GraphQLField,
  GraphQLTypeCategory,
  getCategoryForType,
  getTypeCounts,
  getTypesByCategory,
  graphqlRootCategories,
  graphqlTypeCategories,
  mapGraphQLType,
  parseSchema,
} from "@/lib/schema-helpers";
import { cn } from "@/lib/utils";
import {
  ExclamationTriangleIcon,
  InformationCircleIcon,
} from "@heroicons/react/24/outline";
import { useQuery } from "@tanstack/react-query";
import { EnumStatusCode } from "@wundergraph/cosmo-connect/dist/common/common_pb";
import { getFederatedGraphSDLByName } from "@wundergraph/cosmo-connect/dist/platform/v1/platform-PlatformService_connectquery";
import { sentenceCase } from "change-case";
import { GraphQLSchema } from "graphql";
import Link from "next/link";
import { useRouter } from "next/router";
import { useEffect, useState } from "react";

const Fields = (props: { fields: GraphQLField[]; ast: GraphQLSchema }) => {
  const hasArgs = props.fields.some((f) => !!f.args);

  return (
    <Table className="min-w-[1100px] lg:min-w-full">
      <TableHeader>
        <TableRow>
          <TableHead className="w-3/12">Field</TableHead>
          {hasArgs && <TableHead className="w-4/12">Input</TableHead>}
          <TableHead className="w-2/12">Requests</TableHead>
          <TableHead className="w-1/12">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.fields.map((field) => (
          <TableRow
            className="py-1 even:bg-secondary/30 hover:bg-transparent even:hover:bg-secondary/30"
            key={field.name}
          >
            <TableCell className="align-top font-semibold">
              <p className="mt-2 flex flex-wrap items-center gap-x-1">
                <span>{field.name}</span>
                {field.type && (
                  <TypeLink ast={props.ast} name={`: ${field.type}`} />
                )}
              </p>
            </TableCell>
            {hasArgs && (
              <TableCell className="flex flex-col gap-y-2 py-4">
                {(!field.args || field.args?.length === 0) && <span>-</span>}
                {field.args?.map((arg) => {
                  return (
                    <div
                      key={arg.name}
                      className="flex flex-wrap items-center gap-x-1"
                    >
                      <Tooltip
                        delayDuration={200}
                        open={
                          !arg.description && !arg.deprecationReason
                            ? false
                            : undefined
                        }
                      >
                        <TooltipTrigger>
                          <Badge variant="secondary">{arg.name}</Badge>
                        </TooltipTrigger>
                        <TooltipContent>
                          <div className="flex w-96 flex-col gap-y-4">
                            {arg.description && <p>{arg.description}</p>}
                            {arg.deprecationReason && (
                              <p className="flex flex-col items-start gap-x-1">
                                <span className="flex items-center gap-x-1 font-semibold">
                                  <ExclamationTriangleIcon className="h-3 w-3 flex-shrink-0" />
                                  Deprecated
                                </span>{" "}
                                {arg.deprecationReason}
                              </p>
                            )}
                          </div>
                        </TooltipContent>
                      </Tooltip>
                      <TypeLink ast={props.ast} name={`: ${arg.type}`} />
                    </div>
                  );
                })}
              </TableCell>
            )}
            <TableCell></TableCell>
            <TableCell className="align-top text-primary">
              <Link href={`#`}>
                <p className="mt-2">View Usage</p>
              </Link>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
};

const TypeLink = ({
  name,
  ast,
  isHeading = false,
}: {
  name: string;
  ast: GraphQLSchema;
  isHeading?: boolean;
}) => {
  const router = useRouter();
  const cleanName = name.replace(/[\[\]!: ]/g, "");
  const category = getCategoryForType(ast, cleanName);
  const href =
    router.asPath.split("?")[0] + `?category=${category}&typename=${cleanName}`;

  return (
    <Link href={href}>
      <span
        className={cn(
          "font-semibold text-muted-foreground underline-offset-2 hover:underline",
          {
            "text-xl text-primary-foreground": isHeading,
          }
        )}
      >
        {name}
      </span>
    </Link>
  );
};

const Type = (props: {
  name: string;
  category: GraphQLTypeCategory;
  description: string;
  interfaces?: string[];
  fields?: GraphQLField[];
  ast: GraphQLSchema;
}) => {
  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-x-4">
        <div className="flex items-center gap-x-2 text-xl font-semibold tracking-tight">
          <h3>{props.name}</h3>
          {props.interfaces && props.interfaces.length > 0 && (
            <div className="font-normal text-muted-foreground">implements</div>
          )}
          {props.interfaces &&
            props.interfaces.map((t, index) => (
              <>
                <TypeLink key={index} ast={props.ast} name={t} isHeading />
                {index !== props.interfaces!.length - 1 && (
                  <p className="font-normal text-muted-foreground">&</p>
                )}
              </>
            ))}
        </div>
        <Badge className="w-max">{props.category}</Badge>
      </div>
      <p className="mt-2 text-muted-foreground">{props.description}</p>
      <div className="mt-6">
        {props.fields && <Fields fields={props.fields} ast={props.ast} />}
      </div>
    </div>
  );
};

const TypeWrapper = ({ ast }: { ast: GraphQLSchema }) => {
  const router = useRouter();

  const category = router.query.category as string;
  let typename = router.query.typename as string;

  if (category && !typename) {
    const list = getTypesByCategory(ast, category as GraphQLTypeCategory);
    return (
      <div className="flex flex-col gap-y-2">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Type</TableHead>
              <TableHead>Description</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {list.map((l) => (
              <TableRow key={l.name}>
                <TableCell>
                  <TypeLink ast={ast} name={l.name} />
                </TableCell>
                <TableCell>{l.description || "-"}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    );
  }

  if (!typename) {
    typename = "Query";
  }

  const astType = ast.getType(typename);

  if (!astType)
    return (
      <EmptyState
        className="order-2 h-72 border lg:order-last"
        icon={<InformationCircleIcon />}
        title="No data found"
        description="There is no data for this type or category. Please adjust your filters."
      />
    );

  const type = mapGraphQLType(astType);

  return (
    <div className="mt-2 flex-1">
      <Type
        name={type.name}
        category={type.category}
        description={type.description}
        interfaces={type.interfaces}
        fields={type.fields}
        ast={ast}
      />
    </div>
  );
};

const SchemaExplorerPage: NextPageWithLayout = () => {
  const router = useRouter();
  const graphName = router.query.slug as string;

  const { data, isLoading, error, refetch } = useQuery(
    getFederatedGraphSDLByName.useQuery({
      name: graphName,
    })
  );

  const ast = parseSchema(data?.sdl);

  return (
    <PageHeader title="Studio | SDL">
      <TitleLayout
        title="Schema Explorer"
        subtitle="Explore schema and field level metrics of your federated graph"
        toolbar={<Toolbar ast={ast} />}
      >
        {isLoading && <Loader fullscreen />}
        {!isLoading &&
          (error || data?.response?.code !== EnumStatusCode.OK) && (
            <EmptyState
              icon={<ExclamationTriangleIcon />}
              title="Could not retrieve schema"
              description={
                data?.response?.details || error?.message || "Please try again"
              }
              actions={<Button onClick={() => refetch()}>Retry</Button>}
            />
          )}
        {!isLoading && !ast && (
          <EmptyState
            icon={<ExclamationTriangleIcon />}
            title="Could not retrieve schema"
            description="Chances are the schema is invalid or does not exist"
          />
        )}
        {ast && <TypeWrapper ast={ast} />}
      </TitleLayout>
    </PageHeader>
  );
};

const Toolbar = ({ ast }: { ast: GraphQLSchema | null }) => {
  const router = useRouter();
  const [selectedCategory, setSelectedCategory] = useState("query");

  const typeCounts = ast ? getTypeCounts(ast) : undefined;

  useEffect(() => {
    setSelectedCategory((router.query.category || "query") as string);
  }, [router.query.category]);

  return (
    <SchemaToolbar tab="explorer">
      <Select
        value={selectedCategory}
        onValueChange={(category) => {
          const newQuery = { ...router.query };
          newQuery.category = category;
          if (graphqlRootCategories.includes(category as any)) {
            newQuery.typename = sentenceCase(category);
          } else {
            delete newQuery["typename"];
          }

          router.push({
            query: newQuery,
          });
        }}
      >
        <SelectTrigger className="w-full md:ml-auto md:w-[200px]">
          <SelectValue aria-label={selectedCategory}>
            {sentenceCase(selectedCategory)}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {graphqlRootCategories.map((category) => (
              <SelectItem key={category} value={category}>
                {sentenceCase(category)}
                {typeCounts && (
                  <Badge variant="secondary" className="ml-2">
                    {typeCounts[category]}
                  </Badge>
                )}
              </SelectItem>
            ))}
          </SelectGroup>
          <Separator className="my-2" />
          <SelectGroup>
            {graphqlTypeCategories.map((gType) => {
              return (
                <SelectItem key={gType} value={gType}>
                  <span>{sentenceCase(gType)}</span>
                  {typeCounts && (
                    <Badge variant="secondary" className="ml-2">
                      {typeCounts[gType]}
                    </Badge>
                  )}
                </SelectItem>
              );
            })}
          </SelectGroup>
        </SelectContent>
      </Select>
    </SchemaToolbar>
  );
};

SchemaExplorerPage.getLayout = (page) => getGraphLayout(page);

export default SchemaExplorerPage;
